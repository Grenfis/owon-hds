// OWON HDS272S remote control: oscilloscope, generator and multimeter over USB.
package main

import (
	"context"
	"embed"
	"sync/atomic"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"owon-hds/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	var wctx atomic.Pointer[context.Context]
	a := app.New(func(event string, data any) {
		if ctx := wctx.Load(); ctx != nil {
			runtime.EventsEmit(*ctx, event, data)
		}
	})
	a.SaveDialog = func(name string) (string, error) {
		return runtime.SaveFileDialog(*wctx.Load(), runtime.SaveDialogOptions{DefaultFilename: name})
	}

	err := wails.Run(&options.App{
		Title:            "OWON HDS272S",
		Width:            1400,
		Height:           900,
		MinWidth:         1120,
		MinHeight:        720,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 14, G: 16, B: 21, A: 255},
		OnStartup:        func(ctx context.Context) { wctx.Store(&ctx) },
		OnShutdown:       func(context.Context) { a.Shutdown() },
		Bind:             []any{a},
		Windows:          &windows.Options{Theme: windows.Dark},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
