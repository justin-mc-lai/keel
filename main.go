package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/justin-mc-lai/keel/desktop"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := desktop.New()

	err := wails.Run(&options.App{
		Title:            "Keel",
		Width:            1180,
		Height:           820,
		MinWidth:         860,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 22, G: 22, B: 26, A: 1},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		Bind:             []any{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
