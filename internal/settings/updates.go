// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package settings

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/otnc/Volm/internal/config"
	"github.com/otnc/Volm/internal/update"
)

// updateState is the state of the update page.
type updateState struct {
	busy    bool   // checking or downloading
	message string // the outcome of the last check, for the user
	found   *update.Release
}

// currentVersion is the running Volm's version, or 0.0.0 for a development build without one, so that any release counts as newer.
func currentVersion() update.Version {
	v, err := update.ParseVersion(mygo.App.Version())
	if err != nil {
		return update.Version{}
	}
	return v
}

func (w *window) checkForUpdate() {
	w.updates = updateState{busy: true, message: "確認しています…"}
	prerelease := w.cfg.Update.Prerelease
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rel, err := update.Latest(ctx, prerelease)
		w.win.Update(func() {
			w.updates.busy = false
			switch {
			case errors.Is(err, update.ErrNoRelease):
				w.updates.message = "公開されているリリースが見つかりませんでした。"
			case err != nil:
				log.Printf("settings: checking for updates: %v", err)
				w.updates.message = "更新を確認できませんでした。インターネットに接続しているか確かめてください。"
			case rel.Version.Compare(currentVersion()) > 0:
				w.updates.found = &rel
				w.updates.message = ""
			default:
				w.updates.message = "Volm は最新です。"
			}
		})
	}()
}

// installUpdate downloads the found release's installer, checks it and runs it, then closes this window: the installer replaces Volm.exe, which this process holds open.
func (w *window) installUpdate() {
	rel := *w.updates.found
	w.updates.busy = true
	w.updates.message = "ダウンロードしています…"
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		path, err := update.Download(ctx, rel)
		if err == nil {
			err = update.RunInstaller(path)
		}
		if err != nil {
			log.Printf("settings: installing update: %v", err)
			w.win.Update(func() {
				w.updates.busy = false
				w.updates.message = "更新できませんでした。時間をおいて、もう一度試してください。(" + err.Error() + ")"
			})
			return
		}
		mygo.App.Quit()
	}()
}

func (w *window) updatesPage(c *ui.Context) {
	t := c.Theme()
	def := config.Default()
	pageTitle(c, "更新")
	row(c, w.icons.info, "今のバージョン", "", func() {
		ui.Text(c, mygo.App.Version())
	})
	row(c, w.icons.update, "起動時に更新を確認する", "1 日に 1 回まで確認し、新しいバージョンがあれば通知します。", func() {
		if ui.Switch(c, &w.cfg.Update.CheckOnStartup).Label("起動時に更新を確認する").Changed() {
			w.save()
		}
		w.resetButton(c, "起動時に更新を確認する", w.cfg.Update.CheckOnStartup == def.Update.CheckOnStartup, func() {
			w.cfg.Update.CheckOnStartup = def.Update.CheckOnStartup
		})
	})
	row(c, w.icons.science, "プレリリースも対象にする", "正式リリースの前のバージョンも、新しいバージョンとして知らせます。", func() {
		if ui.Switch(c, &w.cfg.Update.Prerelease).Label("プレリリースも対象にする").Changed() {
			w.save()
		}
		w.resetButton(c, "プレリリースも対象にする", w.cfg.Update.Prerelease == def.Update.Prerelease, func() {
			w.cfg.Update.Prerelease = def.Update.Prerelease
		})
	})

	ui.Row(c).Gap(12).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
		if ui.Button(c, "").Disabled(w.updates.busy).Label("更新を確認する").Children(func() {
			ui.Icon(c, w.icons.update)
			ui.Text(c, "更新を確認").SingleLine()
		}).Clicked() {
			w.checkForUpdate()
		}
		if w.updates.message != "" {
			ui.Text(c, w.updates.message).TextColor(t.TextMuted)
		}
	})

	if rel := w.updates.found; rel != nil {
		ui.Column(c).Gap(10).Padding(16, 18).Radius(10).Background(t.Surface).Border(1, t.Border).Margin(8, 0, 0, 0).Children(func() {
			ui.Text(c, "Volm "+rel.Version.String()+" があります").FontSize(16).Bold()
			if rel.Notes != "" {
				ui.Text(c, rel.Notes).FontSize(12).TextColor(t.TextMuted)
			}
			if ui.PrimaryButton(c, "更新する").Disabled(w.updates.busy).Clicked() {
				w.installUpdate()
			}
			ui.Text(c, "インストーラーをダウンロードして実行し、Volm を入れ替えて起動し直します。この画面は閉じます。").FontSize(12).TextColor(t.TextMuted)
		})
	}
}
