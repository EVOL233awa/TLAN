package control

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/private-radminlan/lanmesh/core"
	"github.com/private-radminlan/lanmesh/internal/storage"
)

type warnCounter struct {
	mu    sync.Mutex
	count int
}

func (c *warnCounter) Enabled(context.Context, slog.Level) bool { return true }
func (c *warnCounter) Handle(_ context.Context, record slog.Record) error {
	if record.Message == "discarding invalid candidates" {
		c.mu.Lock()
		c.count++
		c.mu.Unlock()
	}
	return nil
}
func (c *warnCounter) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *warnCounter) WithGroup(string) slog.Handler      { return c }

// TestCandidatePostAggregatesInvalidLogs pins issue #33: one request with many
// rejected candidates must produce a single log record, not one per candidate.
func TestCandidatePostAggregatesInvalidLogs(t *testing.T) {
	counter := &warnCounter{}
	previous := slog.Default()
	slog.SetDefault(slog.New(counter))
	defer slog.SetDefault(previous)

	store, err := storage.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := &Server{Store: store, AdminToken: "token"}
	h := httptest.NewServer(service.Handler())
	defer h.Close()

	register := core.RegisterDeviceRequest{DeviceID: core.DeviceID("dev-test"), WireGuardPublicKey: "CQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", DisplayName: "Desktop"}
	body, _ := json.Marshal(register)
	req, _ := http.NewRequest(http.MethodPost, h.URL+"/api/v1/devices/register", bytes.NewReader(body))
	req.Header.Set("X-Admin-Token", "token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	network, err := store.CreateNetwork("Private", "network-password", "10.240.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.JoinRegisteredNetwork(core.NetworkID(network.ID), "Desktop", "network-password", core.DeviceID("dev-test")); err != nil {
		t.Fatal(err)
	}

	invalid := strings.TrimSuffix(strings.Repeat(`{"kind":"host","transport":"udp","address":"127.0.0.1:9","priority":1},`, 50), ",")
	candidateBody := `{"candidates":[` + invalid + `]}`
	req, _ = http.NewRequest(http.MethodPost, h.URL+"/api/v1/networks/"+network.ID+"/devices/dev-test/candidates", strings.NewReader(candidateBody))
	req.Header.Set("X-Admin-Token", "token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	counter.mu.Lock()
	defer counter.mu.Unlock()
	if counter.count != 1 {
		t.Fatalf("invalid-candidate log records = %d, want 1 aggregated record", counter.count)
	}
}
