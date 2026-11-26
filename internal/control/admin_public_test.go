package control

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/private-radminlan/lanmesh/core"
	"github.com/private-radminlan/lanmesh/internal/clientapi"
	"github.com/private-radminlan/lanmesh/internal/clientidentity"
	"github.com/private-radminlan/lanmesh/internal/storage"
)

func TestAdminEnrollmentAndClientAdminAPI(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.EnsurePrivateNetwork("Admin Client", "test-join-code", "10.244.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Server{Store: store, AdminToken: "test-admin-token"}).Handler())
	defer server.Close()

	adminIdentity, err := clientidentity.LoadOrCreate(filepath.Join(t.TempDir(), "admin-id.json"))
	if err != nil {
		t.Fatal(err)
	}
	memberAPI := clientapi.New(server.URL, "")
	adminMembership, err := memberAPI.Enroll("test-admin-token", adminIdentity.RegisterRequest(), adminIdentity.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !adminMembership.Admin || adminMembership.Network.ID != network.ID {
		t.Fatalf("admin enrollment = %+v", adminMembership)
	}
	memberAPI.Token = adminMembership.Token
	if _, err := memberAPI.Session(); err != nil {
		t.Fatalf("admin member session failed: %v", err)
	}

	normalIdentity, err := clientidentity.LoadOrCreate(filepath.Join(t.TempDir(), "normal-id.json"))
	if err != nil {
		t.Fatal(err)
	}
	normalAPI := clientapi.New(server.URL, "")
	normalMembership, err := normalAPI.Enroll("test-join-code", normalIdentity.RegisterRequest(), normalIdentity.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	normalAPI.Token = normalMembership.Token
	if normalMembership.Admin {
		t.Fatal("normal enrollment was marked admin")
	}

	adminAPI := clientapi.NewAdmin(server.URL, "test-admin-token")
	overview, err := adminAPI.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.MemberCount != 2 || overview.Network.ID != network.ID {
		t.Fatalf("admin overview = %+v", overview)
	}
	members, err := adminAPI.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members.Members) != 2 {
		t.Fatalf("admin members = %+v", members.Members)
	}
	renamed, err := adminAPI.RenameMember(normalIdentity.DeviceID.String(), "Renamed Member")
	if err != nil || renamed.Name != "Renamed Member" {
		t.Fatalf("rename = %+v, err=%v", renamed, err)
	}
	if err := adminAPI.DisconnectMember(normalIdentity.DeviceID.String()); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if err := adminAPI.KickMember(normalIdentity.DeviceID.String()); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if err := adminAPI.BanMember(normalIdentity.DeviceID.String()); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if _, err := adminAPI.UnbanMember(normalIdentity.DeviceID.String()); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if _, err := clientapi.NewAdmin(server.URL, normalMembership.Token).Overview(); err == nil {
		t.Fatal("member credential accessed public admin API")
	} else {
		var apiErr *clientapi.AdminAPIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
			t.Fatalf("member admin API error = %v", err)
		}
	}
	if _, err := clientapi.NewAdmin(server.URL, "wrong-admin-token").Overview(); err == nil {
		t.Fatal("wrong admin token accessed public admin API")
	}
	if gotRange, err := memberAPI.FRPPortRange(); err != nil || gotRange.Start != 60000 || gotRange.End != 61000 {
		t.Fatalf("default member FRPPortRange = %+v, err=%v", gotRange, err)
	}
	updatedRange, err := adminAPI.SetFRPPortRange(50000, 50500)
	if err != nil || updatedRange.Start != 50000 || updatedRange.End != 50500 {
		t.Fatalf("admin SetFRPPortRange = %+v, err=%v", updatedRange, err)
	}
	if overview2, err := adminAPI.Overview(); err != nil || overview2.FRPPortStart != 50000 || overview2.FRPPortEnd != 50500 {
		t.Fatalf("overview after SetFRPPortRange = %+v, err=%v", overview2, err)
	}
	if gotRange2, err := memberAPI.FRPPortRange(); err != nil || gotRange2.Start != 50000 || gotRange2.End != 50500 {
		t.Fatalf("updated member FRPPortRange = %+v, err=%v", gotRange2, err)
	}
	selfRenamed, err := memberAPI.RenameSelf(core.NetworkID(network.ID), adminIdentity.DeviceID, "PlayerOne")
	if err != nil || selfRenamed.Name != "PlayerOne" {
		t.Fatalf("member RenameSelf = %+v, err=%v", selfRenamed, err)
	}
	if _, err := memberAPI.RenameSelf(core.NetworkID(network.ID), normalIdentity.DeviceID, "Hijacked"); err == nil {
		t.Fatal("member was able to rename another member's device")
	}
}

func TestPublicAdminFailuresRateLimitBadTokensWithoutLockingOutValidToken(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePrivateNetwork("Admin Proxy", "test-join-code", "10.244.0.0/24"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Server{Store: store, AdminToken: "test-admin-token"}).Handler())
	defer server.Close()

	badAPI := clientapi.NewAdmin(server.URL, "wrong-token")
	for i := 0; i < adminLoginLimit; i++ {
		_, err := badAPI.Overview()
		var apiErr *clientapi.AdminAPIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
			t.Fatalf("bad attempt %d err = %v, want 401", i+1, err)
		}
	}
	_, err = badAPI.Overview()
	var apiErr *clientapi.AdminAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
		t.Fatalf("rate-limited bad attempt err = %v, want 429", err)
	}

	// Valid admin token from the same proxy loopback IP must still succeed.
	if _, err := clientapi.NewAdmin(server.URL, "test-admin-token").Overview(); err != nil {
		t.Fatalf("valid admin token was locked out after bad attempts: %v", err)
	}
}

func TestNonAdminPathEnforcesAdminTokenRateLimit(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Server{Store: store, AdminToken: "test-admin-token"}).Handler())
	defer server.Close()

	badAPI := clientapi.New(server.URL, "wrong-admin-token")
	for i := 0; i < adminLoginLimit; i++ {
		if _, err := badAPI.ListNetworks(); err == nil {
			t.Fatalf("attempt %d unexpectedly succeeded", i+1)
		}
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/networks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer wrong-admin-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("non-admin path admin-token status = %d, want 429", resp.StatusCode)
	}
}
