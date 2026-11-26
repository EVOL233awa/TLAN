//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/private-radminlan/lanmesh/core"
)

func TestDefaultInstallDir(t *testing.T) {
	dir := defaultInstallDir()
	if !filepath.IsAbs(dir) || !strings.HasSuffix(dir, "TLAN") {
		t.Fatalf("unexpected default install dir: %q", dir)
	}
}

func TestFormatStartupCommand(t *testing.T) {
	cmd, err := formatStartupCommand(`D:\Apps\TLAN\TLAN.exe`)
	if err != nil {
		t.Fatalf("formatStartupCommand error: %v", err)
	}
	if cmd != `"D:\Apps\TLAN\TLAN.exe" --startup` {
		t.Fatalf("formatStartupCommand = %q", cmd)
	}
	if _, err := formatStartupCommand(`relative\TLAN.exe`); err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestBuildUninstallEntry(t *testing.T) {
	entry := buildUninstallEntry(`D:\Custom\TLAN`)
	if entry.DisplayVersion != core.CurrentVersion {
		t.Fatalf("DisplayVersion = %q, want %q", entry.DisplayVersion, core.CurrentVersion)
	}
	if entry.UninstallString != `"D:\Custom\TLAN\TLAN-Uninstall.exe" --uninstall` {
		t.Fatalf("UninstallString = %q", entry.UninstallString)
	}
}

func TestFormatFixedFileVersion(t *testing.T) {
	got := formatFixedFileVersion((1<<16)|1, (0<<16)|0)
	if got != "1.1.0" {
		t.Fatalf("formatFixedFileVersion = %q, want 1.1.0", got)
	}
}

func TestParseStartupExePath(t *testing.T) {
	if got := parseStartupExePath(`"C:\Program Files\TLAN\TLAN.exe" --startup`); got != `C:\Program Files\TLAN\TLAN.exe` {
		t.Fatalf("parseStartupExePath quoted = %q", got)
	}
	if got := parseStartupExePath(`D:\TLAN\TLAN.exe --startup`); got != `D:\TLAN\TLAN.exe` {
		t.Fatalf("parseStartupExePath unquoted = %q", got)
	}
	if got := parseStartupExePath(""); got != "" {
		t.Fatalf("parseStartupExePath empty = %q", got)
	}
}

func TestCompareReleaseVersions(t *testing.T) {
	if compareReleaseVersions("v1.0.0", "1.1.0") >= 0 {
		t.Fatal("expected v1.0.0 < 1.1.0")
	}
	if compareReleaseVersions("1.2.0", "v1.1.0") <= 0 {
		t.Fatal("expected 1.2.0 > v1.1.0")
	}
	if compareReleaseVersions("v1.1.0", "1.1.0") != 0 {
		t.Fatal("expected v1.1.0 == 1.1.0")
	}
}

func TestFormatExistingInstallPrompt(t *testing.T) {
	existing := existingInstall{Dir: `D:\TLAN`, Version: "1.0.0"}
	titleZH, bodyZH := formatExistingInstallPrompt(existing, "1.1.0", false)
	if !strings.Contains(titleZH, "已安装") || !strings.Contains(bodyZH, "v1.0.0") || !strings.Contains(bodyZH, "v1.1.0") || !strings.Contains(bodyZH, "保留当前电脑版本") {
		t.Fatalf("unexpected ZH prompt: title=%q body=%q", titleZH, bodyZH)
	}
	titleEN, bodyEN := formatExistingInstallPrompt(existingInstall{Dir: `D:\TLAN`, Version: "1.2.0"}, "1.1.0", true)
	if !strings.Contains(titleEN, "Existing") || !strings.Contains(bodyEN, "downgrade") || !strings.Contains(bodyEN, "keep v1.2.0") {
		t.Fatalf("unexpected EN downgrade prompt: title=%q body=%q", titleEN, bodyEN)
	}
}

func TestDetectExistingInstallCandidateAndBuiltPEVersion(t *testing.T) {
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "TLAN.exe")
	if err := os.WriteFile(exe, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, ok := detectExistingInstall(tmp)
	if !ok || found.Dir != tmp {
		t.Fatalf("detectExistingInstall(%q) = (%+v, %v), want dir=%q, ok=true", tmp, found, ok, tmp)
	}

	builtExe := filepath.Join("..", "..", "dist", "windows", "TLAN.exe")
	if _, err := os.Stat(builtExe); err == nil {
		if ver := readPEFileVersion(builtExe); ver != core.CurrentVersion {
			t.Fatalf("readPEFileVersion(%q) = %q, want %q", builtExe, ver, core.CurrentVersion)
		}
	}
}
