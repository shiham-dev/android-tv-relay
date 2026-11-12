package relay

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webFS embed.FS

// webHandler serves the embedded touchscreen UI at the site root.
func webHandler() http.Handler {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err) // embed path is a compile-time constant
	}
	return http.FileServerFS(sub)
}
