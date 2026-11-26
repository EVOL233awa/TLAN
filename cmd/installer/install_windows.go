//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/private-radminlan/lanmesh/core"
	"github.com/private-radminlan/lanmesh/internal/clientidentity"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type existingInstall struct {
	Dir     string
	Version string
}

var (
	versionDLL                 = windows.NewLazySystemDLL("version.dll")
	procGetFileVersionInfoSize = versionDLL.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfo     = versionDLL.NewProc("GetFileVersionInfoW")
	procVerQueryValue          = versionDLL.NewProc("VerQueryValueW")
)

func formatFixedFileVersion(ms, ls uint32) string {
	return fmt.Sprintf("%d.%d.%d", ms>>16, ms&0xffff, ls>>16)
}

func readPEFileVersion(exePath string) string {
	if strings.TrimSpace(exePath) == "" {
		return ""
	}
	if procGetFileVersionInfoSize.Find() != nil || procGetFileVersionInfo.Find() != nil || procVerQueryValue.Find() != nil {
		return ""
	}
	pathPtr := windows.StringToUTF16Ptr(exePath)
	var handle uint32
	sz, _, _ := procGetFileVersionInfoSize.Call(uintptr(unsafe.Pointer(pathPtr)), uintptr(unsafe.Pointer(&handle)))
	if sz == 0 || sz > 1<<20 {
		return ""
	}
	buf := make([]byte, sz)
	ok, _, _ := procGetFileVersionInfo.Call(uintptr(unsafe.Pointer(pathPtr)), 0, sz, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return ""
	}
	subPtr := windows.StringToUTF16Ptr(`\`)
	var fixed *struct {
		Signature     uint32
		StrucVersion  uint32
		FileVersionMS uint32
		FileVersionLS uint32
	}
	var infoLen uint32
	qOk, _, _ := procVerQueryValue.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(subPtr)), uintptr(unsafe.Pointer(&fixed)), uintptr(unsafe.Pointer(&infoLen)))
	if qOk == 0 || fixed == nil || infoLen < 16 {
		runtime.KeepAlive(buf)
		return ""
	}
	sig, ms, ls := fixed.Signature, fixed.FileVersionMS, fixed.FileVersionLS
	runtime.KeepAlive(buf)
	if sig != 0xFEEF04BD {
		return ""
	}
	return formatFixedFileVersion(ms, ls)
}

func parseStartupExePath(cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, `"`) {
		rest := trimmed[1:]
		if idx := strings.IndexByte(rest, '"'); idx > 0 {
			return filepath.Clean(rest[:idx])
		}
		return ""
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return ""
	}
	return filepath.Clean(parts[0])
}

func parseSemverTriplet(raw string) ([3]int, bool) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "v")
	trimmed = strings.TrimPrefix(trimmed, "V")
	if idx := strings.IndexAny(trimmed, "-+"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

func compareReleaseVersions(a, b string) int {
	va, okA := parseSemverTriplet(a)
	vb, okB := parseSemverTriplet(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		if va[i] < vb[i] {
			return -1
		}
		if va[i] > vb[i] {
			return 1
		}
	}
	return 0
}

func detectExistingInstall(extraDirs ...string) (existingInstall, bool) {
	seen := make(map[string]struct{}, len(extraDirs)+1)
	checkDir := func(rawDir string) (existingInstall, bool) {
		trimmed := strings.TrimSpace(rawDir)
		if trimmed == "" {
			return existingInstall{}, false
		}
		dir := filepath.Clean(trimmed)
		key := strings.ToLower(dir)
		if _, exists := seen[key]; exists {
			return existingInstall{}, false
		}
		seen[key] = struct{}{}
		exe := filepath.Join(dir, "TLAN.exe")
		if st, statErr := os.Stat(exe); statErr == nil && !st.IsDir() {
			return existingInstall{Dir: dir, Version: readPEFileVersion(exe)}, true
		}
		return existingInstall{}, false
	}

	for _, rawDir := range extraDirs {
		if found, ok := checkDir(rawDir); ok {
			return found, true
		}
	}

	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallRegKeyPath, registry.QUERY_VALUE); err == nil {
		loc, _, locErr := key.GetStringValue("InstallLocation")
		dispVer, _, _ := key.GetStringValue("DisplayVersion")
		_ = key.Close()
		if locErr == nil && strings.TrimSpace(loc) != "" {
			dir := filepath.Clean(strings.TrimSpace(loc))
			exe := filepath.Join(dir, "TLAN.exe")
			if st, statErr := os.Stat(exe); statErr == nil && !st.IsDir() {
				ver := strings.TrimSpace(dispVer)
				if ver == "" {
					ver = readPEFileVersion(exe)
				}
				return existingInstall{Dir: dir, Version: ver}, true
			}
		}
	}

	if key, err := registry.OpenKey(registry.CURRENT_USER, startupRunKeyPath, registry.QUERY_VALUE); err == nil {
		cmdVal, _, valErr := key.GetStringValue(startupValueName)
		_ = key.Close()
		if valErr == nil {
			if exe := parseStartupExePath(cmdVal); exe != "" {
				if st, statErr := os.Stat(exe); statErr == nil && !st.IsDir() {
					return existingInstall{Dir: filepath.Dir(exe), Version: readPEFileVersion(exe)}, true
				}
			}
		}
	}

	if found, ok := checkDir(defaultInstallDir()); ok {
		return found, true
	}
	return existingInstall{}, false
}

func formatVersionLabel(v string, en bool) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		if en {
			return "Unknown"
		}
		return "未知版本"
	}
	if !strings.HasPrefix(strings.ToLower(trimmed), "v") {
		return "v" + trimmed
	}
	return trimmed
}

func formatExistingInstallPrompt(existing existingInstall, installerVersion string, en bool) (string, string) {
	curLabel := formatVersionLabel(existing.Version, en)
	newLabel := formatVersionLabel(installerVersion, en)
	cmp := compareReleaseVersions(existing.Version, installerVersion)
	var noteZH, noteEN string
	switch {
	case strings.TrimSpace(existing.Version) == "":
		noteZH = "检测到电脑已存在 TLAN 安装目录。"
		noteEN = "An existing TLAN installation was detected on this PC."
	case cmp < 0:
		noteZH = "当前安装包为更新版本，可升级替换现有版本。"
		noteEN = "This installer contains a newer version that can upgrade your existing installation."
	case cmp > 0:
		noteZH = "注意：电脑当前安装的版本高于此安装包（选择继续将降级替换）。"
		noteEN = "Note: The currently installed version is newer than this installer (continuing will downgrade it)."
	default:
		noteZH = "当前已安装相同版本，可覆盖修复安装或保留现有版本。"
		noteEN = "The same version is already installed. You can reinstall over it or keep your current installation."
	}

	if en {
		title := "Existing TLAN Installation Detected"
		body := fmt.Sprintf(
			"Installed version: %s\r\nInstall path: %s\r\nInstaller version: %s\r\n\r\n%s\r\n\r\nClick [Yes] to install %s (replace existing version).\r\nClick [No] to keep %s and cancel setup.",
			curLabel, existing.Dir, newLabel, noteEN, newLabel, curLabel,
		)
		return title, body
	}
	title := "检测到本机已安装 TLAN"
	body := fmt.Sprintf(
		"当前电脑已安装版本：%s\r\n安装路径：%s\r\n本安装包版本：%s\r\n\r\n%s\r\n\r\n点击【是】安装本安装包版本（%s，替换现有版本）；\r\n点击【否】保留当前电脑版本（%s）并取消安装。",
		curLabel, existing.Dir, newLabel, noteZH, newLabel, curLabel,
	)
	return title, body
}

const (
	uninstallRegKeyPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\TLAN`
	startupRunKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupValueName    = "TLAN"
)

type installOptions struct {
	InstallDir      string
	StartMenu       bool
	DesktopShortcut bool
	LaunchAtLogin   bool
	LaunchAfter     bool
}

type uninstallEntry struct {
	DisplayName     string
	DisplayVersion  string
	Publisher       string
	InstallLocation string
	DisplayIcon     string
	UninstallString string
}

func defaultInstallDir() string {
	if pf := strings.TrimSpace(os.Getenv("ProgramFiles")); pf != "" {
		return filepath.Join(pf, "TLAN")
	}
	return `C:\Program Files\TLAN`
}

func formatStartupCommand(exePath string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(exePath))
	if cleaned == "" || !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("install path must be absolute")
	}
	if strings.ContainsAny(cleaned, "\"\x00\r\n") {
		return "", fmt.Errorf("install path contains unsupported characters")
	}
	return `"` + cleaned + `" --startup`, nil
}

func buildUninstallEntry(installDir string) uninstallEntry {
	dir := filepath.Clean(strings.TrimSpace(installDir))
	return uninstallEntry{
		DisplayName:     "TLAN",
		DisplayVersion:  core.CurrentVersion,
		Publisher:       "TLAN",
		InstallLocation: dir,
		DisplayIcon:     filepath.Join(dir, "TLAN.exe"),
		UninstallString: `"` + filepath.Join(dir, "TLAN-Uninstall.exe") + `" --uninstall`,
	}
}

func startMenuDirPath() string {
	base := strings.TrimSpace(os.Getenv("ProgramData"))
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "TLAN")
}

func desktopShortcutPath() string {
	if pub := strings.TrimSpace(os.Getenv("PUBLIC")); pub != "" {
		return filepath.Join(pub, "Desktop", "TLAN.lnk")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Desktop", "TLAN.lnk")
}

func stopRunningTLAN() {
	cmd := exec.Command("taskkill", "/F", "/IM", "TLAN.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}

func psSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func createShortcut(lnkPath, targetPath, workingDir, iconLocation, description string) error {
	if err := os.MkdirAll(filepath.Dir(lnkPath), 0o755); err != nil {
		return err
	}
	script := fmt.Sprintf(
		`$s = (New-Object -ComObject WScript.Shell).CreateShortcut(%s); $s.TargetPath = %s; $s.WorkingDirectory = %s; $s.IconLocation = %s; $s.Description = %s; $s.Save()`,
		psSingleQuote(lnkPath),
		psSingleQuote(targetPath),
		psSingleQuote(workingDir),
		psSingleQuote(iconLocation),
		psSingleQuote(description),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create shortcut %s: %w (%s)", lnkPath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeUninstallRegistry(entry uninstallEntry) error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstallRegKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	for name, val := range map[string]string{
		"DisplayName":     entry.DisplayName,
		"DisplayVersion":  entry.DisplayVersion,
		"Publisher":       entry.Publisher,
		"InstallLocation": entry.InstallLocation,
		"DisplayIcon":     entry.DisplayIcon,
		"UninstallString": entry.UninstallString,
	} {
		if err := key.SetStringValue(name, val); err != nil {
			return err
		}
	}
	if err := key.SetDWordValue("NoModify", 1); err != nil {
		return err
	}
	return key.SetDWordValue("NoRepair", 1)
}

func applyStartupSetting(exePath string, enabled bool) error {
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, startupRunKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
		if err == nil {
			_ = key.DeleteValue(startupValueName)
			_ = key.Close()
		}
	} else {
		cmdStr, err := formatStartupCommand(exePath)
		if err != nil {
			return err
		}
		key, _, err := registry.CreateKey(registry.CURRENT_USER, startupRunKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			return err
		}
		if err := key.SetStringValue(startupValueName, cmdStr); err != nil {
			_ = key.Close()
			return err
		}
		_ = key.Close()
	}

	if settings, err := clientidentity.LoadSettings(); err == nil {
		settings.LaunchAtLogin = enabled
		_ = clientidentity.SaveSettings(settings)
	}
	return nil
}

func installTLAN(opts installOptions, logFn func(string)) error {
	dir := filepath.Clean(strings.TrimSpace(opts.InstallDir))
	if dir == "" || !filepath.IsAbs(dir) {
		return errors.New("invalid installation directory")
	}
	report := func(msg string) {
		if logFn != nil {
			logFn(msg)
		}
	}

	report(tr("正在停止运行中的 TLAN…", "Stopping running TLAN instances…"))
	stopRunningTLAN()

	report(tr("正在复制客户端文件…", "Extracting client files…"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}
	rootFS := payloadFS()
	entries, err := fs.ReadDir(rootFS, ".")
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("installer payload is missing: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src, err := rootFS.Open(entry.Name())
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dir, entry.Name())
		dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("write %s: %w", dstPath, err)
		}
		_, copyErr := io.Copy(dst, src)
		_ = dst.Close()
		_ = src.Close()
		if copyErr != nil {
			return fmt.Errorf("copy %s: %w", entry.Name(), copyErr)
		}
	}

	exePath := filepath.Join(dir, "TLAN.exe")
	uninstallPath := filepath.Join(dir, "TLAN-Uninstall.exe")
	if selfPath, err := os.Executable(); err == nil {
		if data, readErr := os.ReadFile(selfPath); readErr == nil {
			_ = os.WriteFile(uninstallPath, data, 0o755)
		}
	}

	if opts.StartMenu {
		report(tr("正在添加到开始菜单与应用列表…", "Registering Start Menu & Apps list…"))
		lnk := filepath.Join(startMenuDirPath(), "TLAN.lnk")
		if err := createShortcut(lnk, exePath, dir, exePath+",0", "TLAN Virtual LAN "+core.CurrentVersionTag); err != nil {
			return err
		}
	}
	if opts.DesktopShortcut {
		report(tr("正在创建桌面快捷方式…", "Creating Desktop shortcut…"))
		_ = createShortcut(desktopShortcutPath(), exePath, dir, exePath+",0", "TLAN Virtual LAN "+core.CurrentVersionTag)
	}

	report(tr("正在注册系统应用信息…", "Registering Windows Apps & Features entry…"))
	if err := writeUninstallRegistry(buildUninstallEntry(dir)); err != nil {
		return fmt.Errorf("register uninstaller: %w", err)
	}

	report(tr("正在配置开机自启动选项…", "Configuring launch-at-login preference…"))
	if err := applyStartupSetting(exePath, opts.LaunchAtLogin); err != nil {
		return fmt.Errorf("configure launch at login: %w", err)
	}

	if opts.LaunchAfter {
		report(tr("正在启动 TLAN…", "Launching TLAN…"))
		cmd := exec.Command(exePath)
		cmd.Dir = dir
		_ = cmd.Start()
	}
	return nil
}

func uninstallTLAN(installDir string) error {
	stopRunningTLAN()
	_ = os.RemoveAll(startMenuDirPath())
	_ = os.Remove(desktopShortcutPath())
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, uninstallRegKeyPath)
	if key, err := registry.OpenKey(registry.CURRENT_USER, startupRunKeyPath, registry.SET_VALUE); err == nil {
		_ = key.DeleteValue(startupValueName)
		_ = key.Close()
	}

	dir := filepath.Clean(strings.TrimSpace(installDir))
	if dir != "" && filepath.IsAbs(dir) {
		for _, name := range []string{"TLAN.exe", "wintun.dll", "frpc.exe", "tlan-icon.png", "WINTUN-LICENSE.txt", "FRP-LICENSE.txt"} {
			_ = os.Remove(filepath.Join(dir, name))
		}
		self, _ := os.Executable()
		cmd := exec.Command("cmd", "/C", fmt.Sprintf(`ping 127.0.0.1 -n 3 > nul & del /F /Q %q & rmdir %q`, self, dir))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Start()
	}
	return nil
}
