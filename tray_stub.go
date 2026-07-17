//go:build !darwin

package main

// setupTray is a no-op on platforms without the macOS status-bar integration
// (tray.go is darwin-only Objective-C via CGo). Returns a no-op cleanup func.
func (a *App) setupTray() func() {
	return func() {}
}
