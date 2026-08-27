package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var uiAssets embed.FS

func WebAssets() fs.FS {
	root, err := fs.Sub(uiAssets, "web")
	if err != nil {
		panic(err)
	}
	return root
}

func UIHandler() http.Handler {
	root := WebAssets()
	files := http.StripPrefix("/ui/", http.FileServer(http.FS(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// UI assets are embedded under stable names. Caching them would keep an
		// old app.js/style.css alive after a binary upgrade, so the local UI is
		// deliberately always revalidated from the active daemon.
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})
}

func redirectUI(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui/", http.StatusPermanentRedirect)
}
