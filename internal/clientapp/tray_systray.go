//go:build !darwin

package clientapp

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gogpu/systray"

	"github.com/earentir/audiobridge/internal/cli"
)

func runWithTray(ctx context.Context, stop context.CancelFunc, app *App) error {
	go func() {
		if err := app.loop(ctx); err != nil && ctx.Err() == nil {
			log.Printf("client: %v", err)
		}
	}()

	tray := systray.New()
	var trayOnce sync.Once
	shutdown := func() {
		trayOnce.Do(func() {
			stop()
			tray.Remove()
		})
	}

	webURL := cli.WebUIURL(app.cfg.Web.Listen)
	menu := systray.NewMenu()
	menu.Add("Open web UI", func() {
		if err := cli.OpenURL(webURL); err != nil {
			log.Printf("open web ui: %v", err)
		}
	})
	menu.Add("Reconnect", func() {
		app.triggerReconnect()
		tray.ShowNotification("Ears Audio Bridge", "Reconnecting…")
	})
	menu.AddSeparator()
	menu.Add("Quit", func() {
		shutdown()
		os.Exit(0)
	})

	tray.SetIcon(trayIconPNG())
	tray.SetTooltip(trayTooltip(app)).
		SetMenu(menu).
		OnClick(func() {
			if err := cli.OpenURL(webURL); err != nil {
				log.Printf("open web ui: %v", err)
			}
		}).
		Show()

	go watchTrayStatus(ctx, tray, app)

	go func() {
		<-ctx.Done()
		shutdown()
	}()

	if err := tray.Run(); err != nil {
		return fmt.Errorf("tray: %w", err)
	}
	return nil
}

func watchTrayStatus(ctx context.Context, tray *systray.SystemTray, app *App) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	last := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			line := trayTooltip(app)
			tray.SetTooltip(line)
			if status := app.connectionStatus(); status != last {
				last = status
				switch status {
				case "connected":
					tray.ShowNotification("Ears Audio Bridge", "Connected to "+app.cfg.Server)
				case "disconnected":
					if err := app.lastErrorText(); err != "" {
						tray.ShowNotification("Ears Audio Bridge", "Disconnected: "+err)
					}
				}
			}
		}
	}
}

func trayTooltip(app *App) string {
	status, errText := app.traySnapshot()
	line := fmt.Sprintf("Ears Audio Bridge | %s | %s", status, app.cfg.Server)
	if errText != "" && status != "connected" {
		line += " | " + errText
	}
	return line
}

func (a *App) connectionStatus() string {
	status, _ := a.traySnapshot()
	return status
}

func (a *App) lastErrorText() string {
	_, errText := a.traySnapshot()
	return errText
}

func (a *App) traySnapshot() (status, lastErr string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	switch {
	case a.connected:
		status = "connected"
	case a.connecting:
		status = "connecting"
	default:
		status = "disconnected"
	}
	return status, a.lastError
}
