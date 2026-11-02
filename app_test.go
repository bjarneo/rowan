package main

import (
	"context"
	"testing"
)

// Wails calls OnStartup in a goroutine, so the UI can call Bootstrap first.
// Bootstrap must work in that order.
func TestBootstrapBeforeStartup(t *testing.T) {
	t.Setenv("ROWAN_CONFIG_DIR", t.TempDir())
	t.Setenv("ROWAN_CREDENTIALS", "file")
	a := NewApp()
	b := a.Bootstrap()
	if !b.FirstRun || len(b.Config.Connections) != 1 || b.Config.Connections[0].ID != "sample" {
		t.Fatalf("first run %v connections %+v", b.FirstRun, b.Config.Connections)
	}
	if len(b.Config.Snippets) != 4 || b.State.LastConn != "sample" {
		t.Fatalf("snippets %d last conn %q", len(b.Config.Snippets), b.State.LastConn)
	}
	// Before OnStartup there is no window. emit must drop the event.
	a.emit("toast", "no window yet")
	a.shutdown(context.Background())
}
