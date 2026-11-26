// PoC 1: MyGo でウィンドウを開かずにトレイだけで常駐できるかを確かめる。
//
//	go run .
//
// 左クリックで小さな設定画面を開き、閉じても常駐が続くかを見る。
// アイコンは 2 秒ごとに 4 段階を切り替える。右クリックのメニューから終了する。
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type app struct {
	tray      *mygo.Tray
	settings  *mygo.Window
	levelIcon [4][]byte
	level     int
	muted     *mygo.MenuItem
}

func main() {
	mygo.App.SetName("VolmPoC")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := &app{}
	for i := range a.levelIcon {
		a.levelIcon[i] = barIcon(i)
	}
	// リスナーを登録しておかないと、設定画面を閉じた時点でアプリが終了する。
	mygo.App.OnWindowAllClosed(func() { log.Println("all windows closed; staying in tray") })
	mygo.App.OnSecondInstance(func([]string, string) { a.openSettings() })
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) ready() {
	a.muted = &mygo.MenuItem{Label: "ミュート", Type: mygo.MenuItemCheckbox, Click: func(it *mygo.MenuItem, _ *mygo.Window) {
		log.Println("mute:", it.Checked)
	}}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		a.muted,
		{Label: "出力デバイス", Submenu: []*mygo.MenuItem{
			{Label: "スピーカー", Type: mygo.MenuItemRadio, Checked: true},
			{Label: "ヘッドホン", Type: mygo.MenuItemRadio},
		}},
		{Type: mygo.MenuItemSeparator},
		{Label: "設定", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		{Label: "終了", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		ID:      "volm-poc",
		Icon:    a.levelIcon[0],
		ToolTip: "VolmPoC",
		Menu:    menu,
	})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	a.tray.OnClick(a.openSettings)

	go func() {
		for range time.Tick(2 * time.Second) {
			mygo.RunOnMain(a.nextIcon)
		}
	}()
	go logMemory()
}

func (a *app) nextIcon() {
	a.level = (a.level + 1) % len(a.levelIcon)
	if err := a.tray.SetIcon(a.levelIcon[a.level], false); err != nil {
		log.Println("SetIcon:", err)
	}
	a.tray.SetToolTip(fmt.Sprintf("VolmPoC 段階 %d", a.level))
}

func (a *app) openSettings() {
	if a.settings != nil {
		a.settings.Focus()
		return
	}
	a.settings = mygo.NewWindow(mygo.WindowOptions{
		Title:   "VolmPoC 設定",
		Width:   360,
		Height:  200,
		Content: ui.View(func(c *ui.Context) {
			ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
				ui.Text(c, "閉じてもトレイに残るか確認する").FontSize(16).Bold()
				if ui.Button(c, "初期値に戻す").Clicked() {
					log.Println("reset clicked")
				}
			})
		}),
	})
	a.settings.OnClosed(func() {
		a.settings = nil
		// 閉じたあとのメモリが下がるかを見るため、すぐに GC を走らせる。
		runtime.GC()
	})
}

// barIcon は 32x32 の PNG に、level+1 本の縦棒を描く。
func barIcon(level int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	ink := color.NRGBA{0x30, 0x90, 0xff, 0xff}
	for bar := 0; bar <= level; bar++ {
		top := 28 - (bar+1)*6
		for x := 2 + bar*8; x < 8+bar*8; x++ {
			for y := top; y < 30; y++ {
				img.Set(x, y, ink)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func logMemory() {
	for range time.Tick(10 * time.Second) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Fprintf(os.Stderr, "go heap %.1f MB, sys %.1f MB\n", float64(m.HeapAlloc)/1e6, float64(m.Sys)/1e6)
	}
}
