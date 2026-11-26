// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Package settings is the settings window. It runs in a process of its own (volm.exe --settings): a MyGo window keeps about 30MB more memory after it closes, which the resident process should not carry. The window writes config.json on every change, and the resident process watches the file.
package settings

import (
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/otnc/Volm/internal/brightness"
	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/ipc"
)

// WindowTitle is also how a second settings process finds the first one.
const WindowTitle = "Volm の設定"

const (
	pageGeneral    = "general"
	pageVolume     = "volume"
	pageBrightness = "brightness"
	pageHotkeys    = "hotkeys"
	pageOSD        = "osd"
	pageAbout      = "about"
)

// positions pairs the OSD positions with their labels, in menu order.
var positions = []struct{ value, label string }{
	{config.PositionBottomRight, "右下"},
	{config.PositionBottomLeft, "左下"},
	{config.PositionTopRight, "右上"},
	{config.PositionTopLeft, "左上"},
	{config.PositionBottomCenter, "下の中央"},
}

type window struct {
	path string
	cfg  config.Config
	page string

	// Mirrors of cfg in the types the widgets edit.
	volumeStep     float64
	brightnessStep float64
	softDimMin     float64
	osdSeconds     float64
	positionLabel  string

	hotkeys hotkeyEditor
	win     *mygo.Window

	displays     []brightness.Display
	detecting    bool
	methodChoice map[string]string // the label chosen in each monitor's menu

	openAtLogin  bool
	confirmReset bool
	errorText    string
	noticeText   string

	icons    icons
	appImage ui.ImageSource
	logo     ui.ImageSource
}

// Open shows the settings window for the settings file at path. Call it once MyGo is ready; the process ends when the window closes.
func Open(path string, cfg config.Config) {
	w := &window{path: path, page: pageGeneral, icons: loadIcons()}
	w.appImage, _ = ui.DecodeImage(appIconPNG)
	w.logo, _ = ui.DecodeImage(logoPNG)
	w.load(cfg)
	w.openAtLogin = mygo.App.OpenAtLogin()
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     WindowTitle,
		Width:     800,
		Height:    560,
		MinWidth:  640,
		MinHeight: 440,
		Content:   ui.View(w.view),
	})
	w.win = win
	w.detectMonitors(win)
}

func (w *window) load(cfg config.Config) {
	w.cfg = cfg
	w.volumeStep = float64(cfg.Volume.Step)
	w.brightnessStep = float64(cfg.Brightness.Step)
	w.softDimMin = float64(cfg.Brightness.SoftDimMin)
	w.osdSeconds = float64(cfg.OSD.DurationMs) / 1000
	w.positionLabel = positions[0].label
	if i := slices.IndexFunc(positions, func(p struct{ value, label string }) bool { return p.value == cfg.OSD.Position }); i >= 0 {
		w.positionLabel = positions[i].label
	}
}

func (w *window) save() {
	if err := config.Save(w.path, w.cfg); err != nil {
		log.Printf("settings: saving: %v", err)
		w.errorText = "設定を保存できませんでした。" + err.Error()
		return
	}
	w.errorText = ""
}

func (w *window) setOpenAtLogin(on bool) {
	if err := mygo.App.SetOpenAtLogin(on); err != nil {
		log.Printf("settings: open at login: %v", err)
		w.errorText = "Windows の起動時に起動する設定を変更できませんでした。" + err.Error()
	}
	w.openAtLogin = mygo.App.OpenAtLogin()
}

// reset restores every setting, including those of later features that this window does not show yet, and turns starting at login back on.
func (w *window) reset() {
	w.load(config.Default())
	for id := range w.methodChoice {
		w.methodChoice[id] = methodLabel(brightness.MethodAuto)
	}
	w.save()
	w.setOpenAtLogin(true)
}

func (w *window) view(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().Children(func() {
		ui.Column(c).Width(232).FillHeight().Padding(20, 12, 16, 12).Gap(16).Background(t.Surface).Children(func() {
			// The white letters with their indigo outline read on light and dark themes alike.
			if w.logo != nil {
				lw, lh := ui.ImageSize(w.logo)
				ui.Image(c, w.logo).Size(lw/2, lh/2).Margin(0, 0, 0, 8).Label("Volm")
			}
			ui.Sidebar(c, &w.page, func() {
				ui.SidebarItem(c, pageGeneral, w.icons.tune, "一般")
				ui.SidebarItem(c, pageVolume, w.icons.volumeUp, "音量")
				ui.SidebarItem(c, pageBrightness, w.icons.brightness, "明るさ")
				ui.SidebarItem(c, pageHotkeys, w.icons.keyboard, "ホットキー")
				ui.SidebarItem(c, pageOSD, w.icons.osd, "OSD (画面表示)")
				ui.SidebarItem(c, pageAbout, w.icons.info, "情報")
			}).Grow(1)
			ui.Button(c, "").Label("すべての設定を初期値に戻す").Children(func() {
				ui.Icon(c, w.icons.restart)
				ui.Text(c, "初期値に戻す").SingleLine()
			}).OnClick(func() { w.confirmReset = true })
		})
		ui.Scroll(c).Grow(1).FillHeight().Children(func() {
			ui.Column(c).Padding(28, 32).Gap(12).MaxWidth(720).Children(func() {
				if w.errorText != "" {
					ui.Text(c, w.errorText).TextColor(t.Danger)
				}
				if w.noticeText != "" {
					ui.Text(c, w.noticeText).TextColor(t.Success)
				}
				switch w.page {
				case pageGeneral:
					w.generalPage(c)
				case pageVolume:
					w.volumePage(c)
				case pageBrightness:
					w.brightnessPage(c)
				case pageHotkeys:
					w.hotkeysPage(c)
				case pageOSD:
					w.osdPage(c)
				case pageAbout:
					w.aboutPage(c)
				}
			})
		})
	})
	if ui.AlertDialog(c, &w.confirmReset,
		"すべての設定を初期値に戻しますか？",
		"音量、明るさ、ホットキー、OSD の設定がすべて初期値になり、Windows の起動時に起動する設定もオンになります。この操作は取り消せません。",
		"キャンセル", "初期値に戻す") == 1 {
		w.reset()
	}
}

// brandColor is the indigo of Volm's icon tile, lighter on a dark background so that it keeps its contrast. Setting icons use it; controls keep the Windows accent color.
func brandColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGB(0x9C, 0x9A, 0xFF)
	}
	return ui.RGB(0x3F, 0x3C, 0xBB)
}

func pageTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(26).Bold().Margin(0, 0, 8, 0)
}

// row is one setting: an icon, a title with a line of explanation, and the control on the right, on a card.
func row(c *ui.Context, icon *ui.SVG, title, description string, control func()) {
	t := c.Theme()
	ui.Row(c).Padding(14, 18).Gap(16).Radius(10).Background(t.Surface).Border(1, t.Border).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).FontSize(22).TextColor(brandColor(t))
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(14)
			if description != "" {
				ui.Text(c, description).FontSize(12).TextColor(t.TextMuted)
			}
		})
		control()
	})
}

// resetButton is the icon button at the end of a setting that puts that setting alone back to its initial value, without asking: it is easy to change again. It is disabled while the setting already has its initial value.
func (w *window) resetButton(c *ui.Context, title string, isDefault bool, reset func()) {
	if ui.Button(c, "").Label(title + " を初期値に戻す").Tooltip("初期値に戻す").Disabled(isDefault).Padding(6).Children(func() {
		ui.Icon(c, w.icons.restart).FontSize(18)
	}).Clicked() {
		reset()
		// Bring the widgets' mirrors of the settings in line with the reset value.
		w.load(w.cfg)
		w.save()
	}
}

func (w *window) generalPage(c *ui.Context) {
	def := config.Default()
	pageTitle(c, "一般")
	row(c, w.icons.login, "Windows の起動時に Volm を起動する", "サインインしたときに、Volm がトレイに常駐します。", func() {
		if ui.Switch(c, &w.openAtLogin).Label("Windows の起動時に Volm を起動する").Changed() {
			w.setOpenAtLogin(w.openAtLogin)
		}
		// Starting at login is not in the settings file: its initial value is on, as the installer sets it.
		if ui.Button(c, "").Label("Windows の起動時に Volm を起動する を初期値に戻す").Tooltip("初期値に戻す").Disabled(w.openAtLogin).Padding(6).Children(func() {
			ui.Icon(c, w.icons.restart).FontSize(18)
		}).Clicked() {
			w.setOpenAtLogin(true)
		}
	})
	row(c, w.icons.volumeUp, "音量を上げたらミュートを解除する", "ミュート中にホイールで音量を上げると、ミュートも解除します。", func() {
		if ui.Switch(c, &w.cfg.Volume.UnmuteOnIncrease).Label("音量を上げたらミュートを解除する").Changed() {
			w.save()
		}
		w.resetButton(c, "音量を上げたらミュートを解除する", w.cfg.Volume.UnmuteOnIncrease == def.Volume.UnmuteOnIncrease, func() {
			w.cfg.Volume.UnmuteOnIncrease = def.Volume.UnmuteOnIncrease
		})
	})

	ui.Text(c, "Volm の動作").FontSize(16).Bold().Margin(12, 0, 0, 0)
	row(c, w.icons.restartApp, "Volm を再起動する", "常駐している Volm を終了して、起動し直します。この画面は開いたままです。", func() {
		if ui.Button(c, "再起動").Clicked() {
			w.restartResident()
		}
	})
	row(c, w.icons.quit, "Volm を終了する", "常駐している Volm とこの画面を終了します。タスクバーのホイールとホットキーは使えなくなります。", func() {
		if ui.Button(c, "終了").Clicked() {
			w.quitResident()
		}
	})
}

// restartResident asks the resident process to quit and starts a new one. MyGo's instance lock makes the new one wait until the old one has exited.
func (w *window) restartResident() {
	if _, err := ipc.RequestQuit(); err != nil {
		log.Printf("settings: asking Volm to quit: %v", err)
	}
	exe, err := os.Executable()
	if err == nil {
		cmd := exec.Command(exe)
		if err = cmd.Start(); err == nil {
			err = cmd.Process.Release()
		}
	}
	if err != nil {
		log.Printf("settings: starting Volm: %v", err)
		w.errorText = "Volm を起動できませんでした。" + err.Error()
		return
	}
	w.noticeText = "Volm を再起動しました。"
}

// quitResident asks the resident process to quit and closes this window too.
func (w *window) quitResident() {
	if _, err := ipc.RequestQuit(); err != nil {
		log.Printf("settings: asking Volm to quit: %v", err)
	}
	mygo.App.Quit()
}

func (w *window) volumePage(c *ui.Context) {
	def := config.Default()
	pageTitle(c, "音量")
	row(c, w.icons.volumeUp, "タスクバーのホイールで音量を変える", "オフにすると、タスクバーの上のホイールはほかのアプリに渡ります。ホットキーは使えます。", func() {
		if ui.Switch(c, &w.cfg.Volume.WheelEnabled).Label("タスクバーのホイールで音量を変える").Changed() {
			w.save()
		}
		w.resetButton(c, "タスクバーのホイールで音量を変える", w.cfg.Volume.WheelEnabled == def.Volume.WheelEnabled, func() {
			w.cfg.Volume.WheelEnabled = def.Volume.WheelEnabled
		})
	})
	row(c, w.icons.mouse, "ホイール 1 段で変わる音量", "タスクバーの上で 1 段回したときの変化量です。", func() {
		if ui.StepSlider(c, &w.volumeStep, config.MinStep, config.MaxStep, 1).Width(140).Label("ホイール 1 段で変わる音量").Changed() {
			w.cfg.Volume.Step = int(w.volumeStep)
			w.save()
		}
		ui.Text(c, fmt.Sprintf("%d%%", int(w.volumeStep))).Width(44).TextAlign(ui.End)
		w.resetButton(c, "ホイール 1 段で変わる音量", w.cfg.Volume.Step == def.Volume.Step, func() { w.cfg.Volume.Step = def.Volume.Step })
	})
	row(c, w.icons.volumeOff, "中クリックでミュートを切り替える", "タスクバーの上でホイールを押し込むと、ミュートをオンまたはオフにします。", func() {
		if ui.Switch(c, &w.cfg.Volume.MiddleClickMute).Label("中クリックでミュートを切り替える").Changed() {
			w.save()
		}
		w.resetButton(c, "中クリックでミュートを切り替える", w.cfg.Volume.MiddleClickMute == def.Volume.MiddleClickMute, func() {
			w.cfg.Volume.MiddleClickMute = def.Volume.MiddleClickMute
		})
	})
	row(c, w.icons.apps, "Alt+ホイールでアプリごとの音量を変える", "タスクバーのアプリのボタンの上で Alt を押しながら回すと、そのアプリの音量だけが変わります。", func() {
		if ui.Switch(c, &w.cfg.Volume.AppVolumeWithAlt).Label("Alt+ホイールでアプリごとの音量を変える").Changed() {
			w.save()
		}
		w.resetButton(c, "Alt+ホイールでアプリごとの音量を変える", w.cfg.Volume.AppVolumeWithAlt == def.Volume.AppVolumeWithAlt, func() {
			w.cfg.Volume.AppVolumeWithAlt = def.Volume.AppVolumeWithAlt
		})
	})
}

func (w *window) osdPage(c *ui.Context) {
	def := config.Default()
	pageTitle(c, "OSD (画面表示)")
	row(c, w.icons.visibility, "音量を変えたときに OSD を表示する", "操作したモニターに、音量とデバイス名を少しのあいだ表示します。", func() {
		if ui.Switch(c, &w.cfg.OSD.Enabled).Label("音量を変えたときに OSD を表示する").Changed() {
			w.save()
		}
		w.resetButton(c, "音量を変えたときに OSD を表示する", w.cfg.OSD.Enabled == def.OSD.Enabled, func() { w.cfg.OSD.Enabled = def.OSD.Enabled })
	})
	row(c, w.icons.position, "表示する位置", "操作したモニターの中での位置です。", func() {
		labels := make([]string, len(positions))
		for i, p := range positions {
			labels[i] = p.label
		}
		if ui.Select(c, &w.positionLabel, labels).Width(140).Label("表示する位置").Changed() {
			for _, p := range positions {
				if p.label == w.positionLabel {
					w.cfg.OSD.Position = p.value
				}
			}
			w.save()
		}
		w.resetButton(c, "表示する位置", w.cfg.OSD.Position == def.OSD.Position, func() { w.cfg.OSD.Position = def.OSD.Position })
	})
	row(c, w.icons.timer, "表示する時間", "最後の操作から消え始めるまでの時間です。", func() {
		if ui.StepSlider(c, &w.osdSeconds, 0.5, 5, 0.5).Width(140).Label("表示する時間").Changed() {
			w.cfg.OSD.DurationMs = int(w.osdSeconds * 1000)
			w.save()
		}
		ui.Text(c, fmt.Sprintf("%.1f 秒", w.osdSeconds)).Width(52).TextAlign(ui.End)
		w.resetButton(c, "表示する時間", w.cfg.OSD.DurationMs == def.OSD.DurationMs, func() { w.cfg.OSD.DurationMs = def.OSD.DurationMs })
	})
}

func (w *window) aboutPage(c *ui.Context) {
	t := c.Theme()
	pageTitle(c, "情報")
	ui.Row(c).Gap(18).AlignItems(ui.Center).Margin(0, 0, 8, 0).Children(func() {
		if w.appImage != nil {
			ui.Image(c, w.appImage).Size(72, 72)
		}
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "Volm").FontSize(22).Bold()
			ui.Text(c, "バージョン "+mygo.App.Version()).TextColor(t.TextMuted)
			ui.Text(c, "タスクバーの上のホイールで、音量とモニターの明るさを変えるアプリです。").TextColor(t.TextMuted)
		})
	})
	row(c, w.icons.person, "作者", "", func() { ui.Text(c, "otoneko.") })
	row(c, w.icons.gavel, "ライセンス", "GNU General Public License v3.0 only", func() { ui.Text(c, "GPL-3.0-only") })
	row(c, w.icons.code, "ソースコード", "", func() {
		ui.Link(c, "github.com/otnc/Volm", "https://github.com/otnc/Volm")
	})
	ui.Text(c, "使っているライブラリと素材").FontSize(16).Bold().Margin(12, 0, 4, 0)
	ui.Column(c).Padding(6, 18).Radius(10).Background(t.Surface).Border(1, t.Border).Children(func() {
		for i, lib := range thirdParty {
			if i > 0 {
				ui.Divider(c)
			}
			ui.Row(c).PaddingY(10).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, lib.name).Grow(1)
				ui.Text(c, lib.license).FontSize(12).TextColor(t.TextMuted)
			})
		}
	})
	ui.Link(c, "ライセンスの全文を見る", "https://github.com/otnc/Volm/blob/main/THIRD_PARTY_NOTICES.md").Margin(4, 0, 0, 0)
}

// thirdParty matches THIRD_PARTY_NOTICES.md, which tools/gennotices writes. Names and SPDX license expressions stay as their projects write them.
var thirdParty = []struct{ name, license string }{
	{"MyGo", "MIT"},
	{"purego", "Apache-2.0"},
	{"go-ole", "MIT"},
	{"go-text/typesetting", "Unlicense OR BSD-3-Clause"},
	{"go-wca", "MIT"},
	{"golang.org/x/image", "BSD-3-Clause"},
	{"golang.org/x/net", "BSD-3-Clause"},
	{"golang.org/x/sys", "BSD-3-Clause"},
	{"golang.org/x/text", "BSD-3-Clause"},
	{"Go standard library", "BSD-3-Clause"},
	{"Material Symbols", "Apache-2.0"},
}

// The images are drawn by tools/genicons; the logo at twice the size shown.
var (
	//go:embed appicon.png
	appIconPNG []byte
	//go:embed logo.png
	logoPNG []byte
)

//go:embed icons/*.svg
var iconFiles embed.FS

type icons struct {
	tune, volumeUp, volumeOff, osd, info, restart, mouse,
	timer, position, visibility, code, gavel, person, login, restartApp, quit, apps,
	brightness, brightnessLow, monitor,
	keyboard, add, close *ui.SVG
}

// loadIcons parses the Material Symbols (Rounded) icons in icons/.
func loadIcons() icons {
	svg := func(name string) *ui.SVG {
		data, err := iconFiles.ReadFile("icons/" + name + ".svg")
		if err != nil {
			panic(err) // the files are embedded; a missing one is a build mistake
		}
		return ui.MustParseSVG(data)
	}
	return icons{
		tune:          svg("tune"),
		volumeUp:      svg("volume_up"),
		volumeOff:     svg("volume_off"),
		osd:           svg("branding_watermark"),
		info:          svg("info"),
		restart:       svg("restart_alt"),
		mouse:         svg("mouse"),
		timer:         svg("timer"),
		position:      svg("picture_in_picture_alt"),
		brightness:    svg("light_mode"),
		brightnessLow: svg("brightness_low"),
		monitor:       svg("monitor"),
		keyboard:      svg("keyboard"),
		login:         svg("login"),
		restartApp:    svg("autorenew"),
		quit:          svg("power_settings_new"),
		apps:          svg("apps"),
		add:           svg("add"),
		close:         svg("close"),
		visibility:    svg("visibility"),
		code:          svg("code"),
		gavel:         svg("gavel"),
		person:        svg("person"),
	}
}
