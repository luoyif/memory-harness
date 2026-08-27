package main

import (
	"log"

	"github.com/luoyif/memory-harness/internal/server"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

func main() {
	bridge := NewDesktopBridge()
	err := wails.Run(&options.App{
		Title:              "Memory Harness",
		Width:              1440,
		Height:             900,
		MinWidth:           1100,
		MinHeight:          720,
		BackgroundColour:   options.NewRGB(245, 241, 232),
		AssetServer:        &assetserver.Options{Assets: server.WebAssets()},
		OnStartup:          bridge.Startup,
		OnShutdown:         bridge.Shutdown,
		Bind:               []interface{}{bridge},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "com.memory-harness.desktop", OnSecondInstanceLaunch: func(options.SecondInstanceData) { bridge.Show() }},
		Mac:                &mac.Options{TitleBar: mac.TitleBarDefault(), Appearance: mac.NSAppearanceNameAqua, DisableZoom: true},
	})
	if err != nil {
		log.Fatal(err)
	}
}
