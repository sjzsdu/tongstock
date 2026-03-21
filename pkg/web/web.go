package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

func DistFS() http.FileSystem {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

func DistFileServer() http.Handler {
	return http.FileServer(DistFS())
}

func Exists(path string) bool {
	f, err := distFS.Open("dist/" + path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}
