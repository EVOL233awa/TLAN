package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallShConfigTemplateMatchesSchema(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "install.sh")
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	content := string(raw)
	startMarker := `cat >"$tmp_cfg" <<EOF`
	startIdx := strings.Index(content, startMarker)
	if startIdx < 0 {
		t.Fatalf("config heredoc start marker not found in install.sh")
	}
	body := content[startIdx+len(startMarker):]
	endIdx := strings.Index(body, "\nEOF")
	if endIdx < 0 {
		t.Fatalf("config heredoc end marker not found in install.sh")
	}
	template := body[:endIdx]
	replacer := strings.NewReplacer(
		"${NETWORK_NAME}", "TLAN",
		"${NETWORK_CIDR}", "10.240.0.0/24",
		"${JOIN_CODE}", "join-secret-1234",
		"${control_listen}", "127.0.0.1:8080",
		"${STUN_PORT}", "3478",
		"${ADMIN_PORT}", "8081",
		"${DATA_DIR}", "/var/lib/tlan",
		"${ADMIN_TOKEN}", "admin-secret-123456",
	)
	rendered := replacer.Replace(template)
	cfgPath := filepath.Join(t.TempDir(), "tlan-config.yml")
	if err := os.WriteFile(cfgPath, []byte(rendered), 0o600); err != nil {
		t.Fatalf("write rendered config: %v", err)
	}
	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile(install.sh template) failed: %v\nrendered:\n%s", err, rendered)
	}
	if cfg.FRP.Mode != "disabled" {
		t.Fatalf("cfg.FRP.Mode = %q, want disabled", cfg.FRP.Mode)
	}
}
