package main

import (
	"log"

	"github.com/nir0k/GeoRAW/frontend"
	"github.com/nir0k/GeoRAW/internal/gui"
	"github.com/wailsapp/wails/v2"
	wlogger "github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	app := &gui.Backend{}

	err := wails.Run(&options.App{
		Title:       "GeoRAW",
		Width:       1100,
		Height:      900,
		MinWidth:    980,
		MinHeight:   760,
		Windows:     &windows.Options{DisableWindowIcon: false}, // use embedded icon.ico by default
		AssetServer: &assetserver.Options{Assets: frontend.Assets},
		OnStartup:   app.OnStartup,
		Bind:        []interface{}{app},
		LogLevel:    wlogger.ERROR,
	})
	if err != nil {
		log.Fatal(err)
	}
}
