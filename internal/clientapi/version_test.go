package clientapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMajorMinorClientCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Server-Version", "1.1.0")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"network":{"id":"n1","name":"N","cidr":"10.240.0.0/24"},"member":{"id":"d1","name":"P","virtual_ip":"10.240.0.1"}}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "member_tok")
	c.AgentVersion = "v1.0.0"
	if _, err := c.Session(); err == nil || !strings.Contains(err.Error(), "version_mismatch") {
		t.Fatalf("Session with mismatched server version = %v, want version_mismatch", err)
	}

	c.AgentVersion = "v1.1.4"
	if _, err := c.Session(); err != nil {
		t.Fatalf("Session with matching Major.Minor failed: %v", err)
	}
}
