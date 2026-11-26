package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/private-radminlan/lanmesh/core"
	"github.com/private-radminlan/lanmesh/internal/storage"
)

func TestAdminCodeBypassesJoinBudget(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePrivateNetwork("Friends", "friends-secret-code", "10.246.0.0/24"); err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer((&Server{Store: store, AdminToken: "admin-token"}).Handler())
	defer h.Close()

	post := func(code string, device core.RegisterDeviceRequest) int {
		body, err := json.Marshal(map[string]any{"code": code, "device": device})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Post(h.URL+"/api/v1/enroll", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	for i := 0; i < 12; i++ {
		if status := post("wrong-code", core.RegisterDeviceRequest{}); status == http.StatusCreated || status == http.StatusTooManyRequests {
			t.Fatalf("bogus enroll %d status = %d", i, status)
		}
	}
	if status := post("wrong-code", core.RegisterDeviceRequest{}); status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after 12 attempts, got %d", status)
	}
	admin := core.RegisterDeviceRequest{DeviceID: "dev-admin-enroll", WireGuardPublicKey: "CQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
	if status := post("admin-token", admin); status != http.StatusCreated {
		t.Fatalf("admin enrollment after the budget was exhausted = %d, want 201", status)
	}
}
