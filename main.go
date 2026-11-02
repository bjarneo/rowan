package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"

	"rowan/internal/agent"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// devServe is set in builds with the devserve tag. It serves the UI over
// HTTP for headless tests.
var devServe func(app *App, addr string) error

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println("rowan", version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if len(os.Args) > 2 && os.Args[2] == "--stdio" {
			if err := agent.Bridge(os.Stdin, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "rowan:", err)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintln(os.Stderr, "usage: rowan mcp --stdio\n\nForwards MCP messages on stdin and stdout to the running Rowan app.")
		os.Exit(2)
	}

	app := NewApp()
	if devServe != nil && len(os.Args) > 2 && os.Args[1] == "--serve" {
		if err := devServe(app, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "rowan:", err)
			os.Exit(1)
		}
		return
	}
	bg := &options.RGBA{R: 250, G: 250, B: 248, A: 255}
	if app.cfg.Get().App.Theme == "dark" {
		bg = &options.RGBA{R: 21, G: 22, B: 25, A: 255}
	}
	err := wails.Run(&options.App{
		Title:            "Rowan",
		Width:            1360,
		Height:           860,
		MinWidth:         960,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: bg,
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []any{app},
		Linux: &linux.Options{
			Icon:             icon,
			ProgramName:      "rowan",
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rowan:", err)
		os.Exit(1)
	}
}
