// ArkPeak is a cozy procedural town-builder: a homage to Townscaper by
// Oskar Stålberg.
//
// The Go backend owns the Wave Function Collapse solver and the authoritative
// town state. The TypeScript frontend owns rendering, input and camera, and
// calls the TownService through the bindings Wails generates.
package main

import (
	"embed"
	"log"
	"math/rand"
	"time"

	"github.com/incurl/arkpeak/internal/town"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the
// binary. Any files in the frontend/dist folder will be embedded into the
// binary and made available to the frontend.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	rules, err := town.LoadEmbeddedRules()
	if err != nil {
		log.Fatalf("arkpeak: %v", err)
	}

	// The seed is randomised per launch so every session starts with a
	// different town. It is stored in the save file, so a loaded town is
	// always reproduced exactly.
	seed := time.Now().UnixNano() ^ rand.Int63()

	service := town.NewService(rules, seed)

	app := application.New(application.Options{
		Name:        "ArkPeak",
		Description: "A cozy procedural town-builder",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "ArkPeak",
		Width:  1280,
		Height: 800,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(150, 180, 190),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
