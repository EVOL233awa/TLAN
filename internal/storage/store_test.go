package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/private-radminlan/lanmesh/core"
)

func TestFailedPersistRollsBackMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateNetwork("first", "a valid password", "10.240.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(path, "cannot-exist.json")
	if _, err = store.CreateNetwork("must not commit", "a valid password", "10.241.0.0/24"); err == nil {
		t.Fatal("expected persistence failure")
	}
	if len(store.ListNetworks()) != 1 {
		t.Fatal("failed disk write was committed in memory")
	}
	store.path = path
	key := "CQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	_, _, err = store.RegisterDevice(core.RegisterDeviceRequest{DeviceID: "test-device", WireGuardPublicKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.JoinRegisteredNetwork(core.NetworkID(first.ID), "member", "a valid password", "test-device"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.NetworkMembershipExists(core.NetworkID(first.ID), "test-device") {
		t.Fatal("state was not persisted after rollback")
	}
}

func TestFRPPortRangeDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.HasFRPPortRange() {
		t.Fatal("expected unset custom FRP port range by default")
	}
	got := store.FRPPortRange()
	if got != (PortRange{Start: 60000, End: 61000}) {
		t.Fatalf("default FRPPortRange = %+v, want 60000-61000", got)
	}
	if _, err := store.SetFRPPortRange(70000, 70001); err == nil {
		t.Fatal("expected invalid port range rejection")
	}
	if _, err := store.SetFRPPortRange(50010, 50000); err == nil {
		t.Fatal("expected reversed port range rejection")
	}
	saved, err := store.SetFRPPortRange(60100, 60200)
	if err != nil || saved != (PortRange{Start: 60100, End: 60200}) {
		t.Fatalf("SetFRPPortRange = %+v, err=%v", saved, err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.HasFRPPortRange() || reopened.FRPPortRange() != saved {
		t.Fatalf("reopened FRPPortRange = %+v (has=%v), want %+v", reopened.FRPPortRange(), reopened.HasFRPPortRange(), saved)
	}
}

func TestEnrollPrivateRotatesRandomTokenOnRevokeAndReenroll(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.EnsurePrivateNetwork("friends", "friends-code", "10.243.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	req := core.RegisterDeviceRequest{
		DeviceID:           "dev-token-rotate",
		WireGuardPublicKey: "CQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}
	_, _, firstToken, err := store.EnrollPrivate(req, "friends-code")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.AuthenticateMember(firstToken); !ok {
		t.Fatal("first token should authenticate")
	}
	if err := store.RevokeMemberCredentials(n.ID, req.DeviceID.String()); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.AuthenticateMember(firstToken); ok {
		t.Fatal("revoked token must be rejected")
	}
	_, _, secondToken, err := store.EnrollPrivate(req, "friends-code")
	if err != nil {
		t.Fatal(err)
	}
	if secondToken == firstToken {
		t.Fatal("re-enrolling issued identical token; revoked token would resurrect")
	}
	if _, ok := store.AuthenticateMember(firstToken); ok {
		t.Fatal("old revoked token must remain rejected after re-enrollment")
	}
	if _, ok := store.AuthenticateMember(secondToken); !ok {
		t.Fatal("newly issued token should authenticate")
	}
}

func TestHeartbeatAndUnchangedCandidatesThrottleDiskWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.EnsurePrivateNetwork("friends", "friends-code", "10.243.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = store.EnrollPrivate(core.RegisterDeviceRequest{
		DeviceID:           "dev-throttle",
		WireGuardPublicKey: "CQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}, "friends-code")
	if err != nil {
		t.Fatal(err)
	}
	// First heartbeat transitions online and persists.
	if _, err := store.Heartbeat("dev-throttle"); err != nil {
		t.Fatal(err)
	}
	cands := []core.Candidate{{Kind: core.CandidateHost, Transport: core.TransportUDP, Address: "198.51.100.20:4000"}}
	if _, err := store.SetCandidates(core.NetworkID(n.ID), "dev-throttle", cands); err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Ensure clock moves past filesystem timestamp granularity if needed, without writing.
	time.Sleep(20 * time.Millisecond)
	if _, err := store.Heartbeat("dev-throttle"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetCandidates(core.NetworkID(n.ID), "dev-throttle", cands); err != nil {
		t.Fatal(err)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("expected steady heartbeat and unchanged candidates not to rewrite %s", path)
	}

	// Reject oversized or control-character client versions.
	if err := store.SetClientVersion("dev-throttle", strings.Repeat("1", 100)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetClientVersion("dev-throttle", "1.0.0\nbad"); err != nil {
		t.Fatal(err)
	}
	if v, ok := store.ClientVersion("dev-throttle"); !ok || v != "" {
		t.Fatalf("invalid client version persisted: %q", v)
	}
}
