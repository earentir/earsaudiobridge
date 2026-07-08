//go:build darwin

package clientapp

import (
	"context"
	"log"
)

// runWithTray on macOS: gogpu/systray (goffi) and oto (purego) both ship a fakecgo
// stub that collides when CGO_ENABLED=0, so tray is unavailable in static builds.
func runWithTray(ctx context.Context, stop context.CancelFunc, app *App) error {
	log.Print("system tray is not available on macOS static builds; use the web UI at " + app.cfg.Web.Listen)
	return app.loop(ctx)
}
