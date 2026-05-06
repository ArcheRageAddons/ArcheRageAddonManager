package main

import (
	"embed"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// Version is injected at build time via -ldflags "-X main.Version=v0.4.0".
// In dev mode (running under `wails dev`) it stays "dev" and the
// update-check loop is skipped.
var Version = "dev"

// Audit #11: Content-Security-Policy for the embedded webview. The frontend
// only ever calls Go via Wails IPC (postMessage-based, not HTTP), so
// connect-src 'self' is enough — no fetches go to the network from JS. img-src
// permits any https origin because addon icons and Discord/GitHub avatars
// come from arbitrary CDN hosts (raw.githubusercontent.com, cdn.discordapp.com,
// gravatar, etc.), but anything else is locked to 'self'. 'unsafe-inline' is
// kept for style-src because Svelte/Tailwind emit inline <style> blocks; we
// don't allow it for script-src — Vite ships ES modules, not inline scripts.
const contentSecurityPolicy = "" +
	"default-src 'self'; " +
	"script-src 'self' 'wasm-unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'none';"

func cspMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "ArcheRage Addon Manager",
		Width:     1100,
		Height:    700,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: cspMiddleware,
		},
		BackgroundColour: &options.RGBA{R: 26, G: 26, B: 46, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
