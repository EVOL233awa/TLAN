//go:build windows

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/private-radminlan/lanmesh/core"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var isEnglish bool

func init() {
	dll := windows.NewLazySystemDLL("kernel32.dll")
	proc := dll.NewProc("GetUserDefaultUILanguage")
	if proc.Find() == nil {
		langID, _, _ := proc.Call()
		primary := uint16(langID) & 0x3ff
		isEnglish = primary != 0x04 // LANG_CHINESE
	}
}

func tr(zh, en string) string {
	if isEnglish {
		return en
	}
	return zh
}

func main() {
	uninstallFlag := flag.Bool("uninstall", false, "uninstall TLAN")
	flag.Parse()

	if *uninstallFlag {
		dir := ""
		if key, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallRegKeyPath, registry.QUERY_VALUE); err == nil {
			if loc, _, err := key.GetStringValue("InstallLocation"); err == nil {
				dir = loc
			}
			_ = key.Close()
		}
		if dir == "" {
			if self, err := os.Executable(); err == nil {
				dir = filepath.Dir(self)
			}
		}
		if walk.MsgBox(nil, tr("卸载 TLAN", "Uninstall TLAN"), tr("确定要卸载 TLAN 并移除快捷方式与开机启动项吗？", "Uninstall TLAN and remove shortcuts and startup entries?"), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			return
		}
		if err := uninstallTLAN(dir); err != nil {
			walk.MsgBox(nil, tr("卸载失败", "Uninstall Failed"), err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
			return
		}
		walk.MsgBox(nil, tr("卸载完成", "Uninstalled"), tr("TLAN 已卸载完成。", "TLAN has been uninstalled."), walk.MsgBoxOK|walk.MsgBoxIconInformation)
		return
	}

	var mw *walk.MainWindow
	var dirEdit *walk.LineEdit
	var startMenuCB, desktopCB, startupCB, launchAfterCB *walk.CheckBox
	var headingLbl, subtitleLbl, existingLbl, dirLbl, statusLbl *walk.Label
	var langBtn, browseBtn, installBtn, cancelBtn *walk.PushButton
	installing := false

	existing, hasExisting := detectExistingInstall()
	initialDir := defaultInstallDir()
	if hasExisting && existing.Dir != "" {
		initialDir = existing.Dir
	}

	existingBannerText := func() string {
		if !hasExisting {
			return ""
		}
		ver := formatVersionLabel(existing.Version, isEnglish)
		if isEnglish {
			return "Existing install detected: TLAN " + ver + " (" + existing.Dir + ")"
		}
		return "检测到本机已安装 TLAN " + ver + "（" + existing.Dir + "）"
	}

	retranslate := func() {
		if mw != nil {
			_ = mw.SetTitle(tr("TLAN "+core.CurrentVersionTag+" 安装程序", "TLAN "+core.CurrentVersionTag+" Setup"))
		}
		if langBtn != nil {
			_ = langBtn.SetText(tr("English", "简体中文"))
		}
		if headingLbl != nil {
			_ = headingLbl.SetText(tr("安装 TLAN "+core.CurrentVersionTag+" 私有虚拟局域网客户端", "Install TLAN "+core.CurrentVersionTag+" Virtual LAN Client"))
		}
		if subtitleLbl != nil {
			_ = subtitleLbl.SetText(tr("将完整安装 TLAN 主程序、Wintun 驱动组件与内置端口映射引擎，并注册至“最近添加”与系统应用列表。", "Installs TLAN, Wintun driver files, and built-in FRP engine, and registers Start Menu / Recently Added & Windows Apps."))
		}
		if existingLbl != nil && hasExisting {
			_ = existingLbl.SetText(existingBannerText())
		}
		if dirLbl != nil {
			_ = dirLbl.SetText(tr("安装目录", "Install Location"))
		}
		if browseBtn != nil {
			_ = browseBtn.SetText(tr("浏览…", "Browse…"))
		}
		if startMenuCB != nil {
			_ = startMenuCB.SetText(tr("添加到开始菜单（最近添加）与系统应用列表", "Add to Start Menu (Recently Added) & Apps list"))
		}
		if desktopCB != nil {
			_ = desktopCB.SetText(tr("创建桌面快捷方式", "Create Desktop shortcut"))
		}
		if startupCB != nil {
			_ = startupCB.SetText(tr("开机自动启动 TLAN（静默驻留托盘）", "Launch TLAN automatically at Windows startup"))
		}
		if launchAfterCB != nil {
			_ = launchAfterCB.SetText(tr("安装完成后立即启动 TLAN", "Launch TLAN immediately after installation"))
		}
		if cancelBtn != nil {
			_ = cancelBtn.SetText(tr("取消", "Cancel"))
		}
		if installBtn != nil {
			_ = installBtn.SetText(tr("立即安装", "Install"))
		}
	}

	icon, _ := walk.NewIconFromResource("APP")
	if icon != nil {
		defer icon.Dispose()
	}

	err := (MainWindow{
		AssignTo: &mw,
		Title:    tr("TLAN "+core.CurrentVersionTag+" 安装程序", "TLAN "+core.CurrentVersionTag+" Setup"),
		Icon:     icon,
		Size:     Size{Width: 540, Height: 375},
		MinSize:  Size{Width: 500, Height: 350},
		MaxSize:  Size{Width: 660, Height: 450},
		Font:     Font{Family: "Microsoft YaHei UI", PointSize: 9},
		Layout:   VBox{Margins: Margins{Left: 22, Top: 18, Right: 22, Bottom: 18}, Spacing: 11},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					Label{
						AssignTo:      &headingLbl,
						StretchFactor: 1,
						Text:          tr("安装 TLAN "+core.CurrentVersionTag+" 私有虚拟局域网客户端", "Install TLAN "+core.CurrentVersionTag+" Virtual LAN Client"),
						Font:          Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
						NoPrefix:      true,
					},
					PushButton{
						AssignTo: &langBtn,
						Text:     tr("English", "简体中文"),
						MinSize:  Size{Width: 78},
						OnClicked: func() {
							if installing {
								return
							}
							isEnglish = !isEnglish
							retranslate()
						},
					},
				},
			},
			Label{
				AssignTo: &subtitleLbl,
				Text:     tr("将完整安装 TLAN 主程序、Wintun 驱动组件与内置端口映射引擎，并注册至“最近添加”与系统应用列表。", "Installs TLAN, Wintun driver files, and built-in FRP engine, and registers Start Menu / Recently Added & Windows Apps."),
				NoPrefix: true,
			},
			Label{
				AssignTo:  &existingLbl,
				Visible:   hasExisting,
				Text:      existingBannerText(),
				TextColor: walk.RGB(37, 99, 235),
				Font:      Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true},
				NoPrefix:  true,
			},
			Composite{
				Layout: VBox{MarginsZero: true, Spacing: 5},
				Children: []Widget{
					Label{AssignTo: &dirLbl, Text: tr("安装目录", "Install Location"), Font: Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true}, NoPrefix: true},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							LineEdit{AssignTo: &dirEdit, Text: initialDir, StretchFactor: 1},
							PushButton{
								AssignTo: &browseBtn,
								Text:     tr("浏览…", "Browse…"),
								MinSize:  Size{Width: 82},
								OnClicked: func() {
									dlg := walk.FileDialog{
										Title:          tr("选择 TLAN 安装文件夹", "Select TLAN Install Folder"),
										InitialDirPath: strings.TrimSpace(dirEdit.Text()),
									}
									if ok, _ := dlg.ShowBrowseFolder(mw); ok && dlg.FilePath != "" {
										_ = dirEdit.SetText(dlg.FilePath)
									}
								},
							},
						},
					},
				},
			},
			Composite{
				Layout: VBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					CheckBox{AssignTo: &startMenuCB, Checked: true, Text: tr("添加到开始菜单（最近添加）与系统应用列表", "Add to Start Menu (Recently Added) & Apps list")},
					CheckBox{AssignTo: &desktopCB, Checked: true, Text: tr("创建桌面快捷方式", "Create Desktop shortcut")},
					CheckBox{AssignTo: &startupCB, Checked: false, Text: tr("开机自动启动 TLAN（静默驻留托盘）", "Launch TLAN automatically at Windows startup")},
					CheckBox{AssignTo: &launchAfterCB, Checked: true, Text: tr("安装完成后立即启动 TLAN", "Launch TLAN immediately after installation")},
				},
			},
			VSpacer{},
			Label{AssignTo: &statusLbl, Text: "", NoPrefix: true},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &cancelBtn,
						Text:     tr("取消", "Cancel"),
						MinSize:  Size{Width: 88},
						OnClicked: func() {
							if !installing {
								mw.Close()
							}
						},
					},
					PushButton{
						AssignTo: &installBtn,
						Text:     tr("立即安装", "Install"),
						MinSize:  Size{Width: 110},
						OnClicked: func() {
							if installing {
								return
							}
							selectedDir := strings.TrimSpace(dirEdit.Text())
							if found, ok := detectExistingInstall(selectedDir); ok {
								promptTitle, promptBody := formatExistingInstallPrompt(found, core.CurrentVersion, isEnglish)
								if walk.MsgBox(mw, promptTitle, promptBody, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
									return
								}
							}
							installing = true
							installBtn.SetEnabled(false)
							cancelBtn.SetEnabled(false)
							browseBtn.SetEnabled(false)
							langBtn.SetEnabled(false)
							opts := installOptions{
								InstallDir:      selectedDir,
								StartMenu:       startMenuCB.Checked(),
								DesktopShortcut: desktopCB.Checked(),
								LaunchAtLogin:   startupCB.Checked(),
								LaunchAfter:     launchAfterCB.Checked(),
							}
							go func() {
								instErr := installTLAN(opts, func(msg string) {
									mw.Synchronize(func() {
										_ = statusLbl.SetText(msg)
									})
								})
								mw.Synchronize(func() {
									installing = false
									if instErr != nil {
										installBtn.SetEnabled(true)
										cancelBtn.SetEnabled(true)
										browseBtn.SetEnabled(true)
										langBtn.SetEnabled(true)
										_ = statusLbl.SetText("")
										walk.MsgBox(mw, tr("安装失败", "Installation Failed"), instErr.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
										return
									}
									walk.MsgBox(mw, tr("安装完成", "Installation Complete"), tr("TLAN "+core.CurrentVersionTag+" 已成功安装！", "TLAN "+core.CurrentVersionTag+" has been installed successfully!"), walk.MsgBoxOK|walk.MsgBoxIconInformation)
									mw.Close()
								})
							}()
						},
					},
				},
			},
		},
	}).Create()
	if err != nil {
		walk.MsgBox(nil, "TLAN Setup", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return
	}
	mw.Run()
}
