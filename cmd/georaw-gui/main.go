package main

import (
	"log"

	"github.com/nir0k/GeoRAW/frontend"
	"github.com/nir0k/GeoRAW/internal/gui"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wlogger "github.com/wailsapp/wails/v2/pkg/logger"
)

func main() {
	app := &gui.Backend{}

	err := wails.Run(&options.App{
		Title:       "GeoRAW",
		Width:       960,
		Height:      720,
		AssetServer: &assetserver.Options{Assets: frontend.Assets},
		OnStartup:   app.OnStartup,
		Bind:        []interface{}{app},
		LogLevel:    wlogger.ERROR,
	})
	if err != nil {
		log.Fatal(err)
	}
}
