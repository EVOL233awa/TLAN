package mesh

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/private-radminlan/lanmesh/internal/clientapi"
	"github.com/private-radminlan/lanmesh/internal/clientidentity"
	"github.com/private-radminlan/lanmesh/internal/network"
)

func TestRelayBackoffSuppressesImmediateRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "temporary"})
	}))
	defer server.Close()

	peer := clientapi.Device{ID: "dev-peer", VirtualIP: "10.242.0.2"}
	ip := netip.MustParseAddr(peer.VirtualIP)
	engine := &Engine{
		options: Options{
			API:      clientapi.New(server.URL, "member_test"),
			Identity: clientidentity.Identity{DeviceID: "dev-self"},
			Network:  clientapi.Network{ID: "net-test"},
		},
		ctx:     context.Background(),
		dialing: make(map[string]bool),
		peers:   map[string]clientapi.Device{peer.ID: peer},
		bind:    &meshBind{routes: map[netip.Addr]*peerRoute{ip: {}}},
	}

	engine.ensureRelay(peer, ip)
	waitForRelayIdle(t, engine, peer.ID)
	engine.ensureRelay(peer, ip)
	time.Sleep(100 * time.Millisecond)
	if got := attempts.Load(); got != 1 {
		t.Fatalf("relay attempts = %d, want 1 while backing off", got)
	}
}

func TestEnsureRelayRefreshesNearExpiry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "temporary"})
	}))
	defer server.Close()

	peer := clientapi.Device{ID: "dev-peer", VirtualIP: "10.242.0.2"}
	ip := netip.MustParseAddr(peer.VirtualIP)
	engine := &Engine{
		options: Options{
			API:      clientapi.New(server.URL, "member_test"),
			Identity: clientidentity.Identity{DeviceID: "dev-self"},
			Network:  clientapi.Network{ID: "net-test"},
		},
		ctx:     context.Background(),
		dialing: make(map[string]bool),
		peers:   map[string]clientapi.Device{peer.ID: peer},
		bind: &meshBind{routes: map[netip.Addr]*peerRoute{ip: {
			relay:       &network.StreamRelay{},
			relayExpiry: time.Now().Add(20 * time.Second),
		}}},
	}

	engine.ensureRelay(peer, ip)
	deadline := time.Now().Add(time.Second)
	for attempts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("relay attempts = %d, want rebuild before expiry", got)
	}
}

func TestRelayBackoffSequenceAndSuccessReset(t *testing.T) {
	engine := &Engine{}
	now := time.Unix(100, 0)
	peerID := "dev-peer"

	engine.relayAttemptFailed(peerID, now)
	if engine.relayAttemptAllowed(peerID, now.Add(999*time.Millisecond)) {
		t.Fatal("peer retried before the first backoff elapsed")
	}
	if !engine.relayAttemptAllowed(peerID, now.Add(time.Second)) {
		t.Fatal("peer did not become eligible after 1s")
	}

	second := now.Add(time.Second)
	engine.relayAttemptFailed(peerID, second)
	if engine.relayAttemptAllowed(peerID, second.Add(1999*time.Millisecond)) {
		t.Fatal("peer retried before the second backoff elapsed")
	}
	if !engine.relayAttemptAllowed(peerID, second.Add(2*time.Second)) {
		t.Fatal("peer did not become eligible after 2s")
	}

	engine.relayAttemptSucceeded(peerID)
	if !engine.relayAttemptAllowed(peerID, second) {
		t.Fatal("success did not clear relay backoff")
	}
}

func TestRelayBackoffCapsAndPeerCleanup(t *testing.T) {
	cases := []struct {
		failures uint32
		want     time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{6, 30 * time.Second},
		{20, 30 * time.Second},
	}
	for _, test := range cases {
		if got := relayBackoffDelay(test.failures); got != test.want {
			t.Fatalf("relayBackoffDelay(%d) = %s, want %s", test.failures, got, test.want)
		}
	}

	engine := &Engine{}
	now := time.Unix(200, 0)
	engine.relayAttemptFailed("dev-peer", now)
	engine.clearRelayAttempt("dev-peer")
	if !engine.relayAttemptAllowed("dev-peer", now) {
		t.Fatal("peer cleanup did not clear relay backoff")
	}
}

func waitForRelayIdle(t *testing.T, engine *Engine, peerID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		engine.mu.RLock()
		dialing := engine.dialing[peerID]
		engine.mu.RUnlock()
		if !dialing {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("relay attempt did not finish")
}

func TestSyncPeersSkipsBadPeerInsteadOfAborting(t *testing.T) {
	selfID, err := clientidentity.LoadOrCreate(filepath.Join(t.TempDir(), "self.json"))
	if err != nil {
		t.Fatal(err)
	}
	goodID, err := clientidentity.LoadOrCreate(filepath.Join(t.TempDir(), "good.json"))
	if err != nil {
		t.Fatal(err)
	}
	zeroKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	devices := []clientapi.Device{
		{ID: selfID.DeviceID.String(), PublicKey: selfID.PublicKey, VirtualIP: "10.242.0.1"},
		{ID: "dev-bad-zero", PublicKey: zeroKey, VirtualIP: "10.242.0.2"},
		{ID: goodID.DeviceID.String(), PublicKey: goodID.PublicKey, VirtualIP: "10.242.0.3"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/networks/net-test/devices" {
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": devices})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/networks/net-test/devices/"+selfID.DeviceID.String()+"/candidates" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	nic := newNIC()
	engine, err := Start(context.Background(), Options{
		API:       clientapi.New(server.URL, "member_test"),
		Identity:  selfID,
		Network:   clientapi.Network{ID: "net-test", Name: "test", CIDR: "10.242.0.0/24"},
		Member:    clientapi.Device{ID: selfID.DeviceID.String(), VirtualIP: "10.242.0.1"},
		Native:    nic,
		RelayOnly: true,
	})
	if err != nil {
		t.Fatalf("Start() failed despite valid peer: %v", err)
	}
	defer engine.Close()

	engine.mu.RLock()
	_, hasGood := engine.peers[goodID.DeviceID.String()]
	_, hasBad := engine.peers["dev-bad-zero"]
	engine.mu.RUnlock()
	if !hasGood || hasBad {
		t.Fatalf("peers state: hasGood=%t hasBad=%t, want true/false", hasGood, hasBad)
	}
}
