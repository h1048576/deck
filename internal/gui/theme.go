package gui

import (
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func serveHTTP(addr string, handler http.Handler) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}

func themeRGBA(theme string) application.RGBA {
	if theme == "light" {
		return application.NewRGBA(0xFF, 0xFF, 0xFF, 1)
	}
	return application.NewRGBA(0x10, 0x10, 0x10, 1)
}

func themeBackground(string) string { return "" }
