//go:build !darwin

package main

// hasTray reports whether this platform has a status-bar tray to hide into.
// Owned by the tray build-tag pair so main.go's hide-on-close behavior can
// never drift from the tray implementation (see background-presence epic for
// the plan to bring a tray to the remaining platforms).
const hasTray = false

// setupTray is a no-op on platforms without the macOS status-bar integration
// (tray.go is darwin-only Objective-C via CGo). Returns a no-op cleanup func.
func (a *App) setupTray() func() {
	return func() {}
}
