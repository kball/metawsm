package web

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
)

type SPAOptions struct {
	APIPrefix string
	WSPath    string
}

func RegisterSPA(mux *http.ServeMux, publicFS fs.FS, options SPAOptions) bool {
	if publicFS == nil {
		return false
	}
	if _, err := publicFS.Open("index.html"); err != nil {
		return false
	}

	apiPrefix := options.APIPrefix
	if apiPrefix == "" {
		apiPrefix = "/api"
	}
	wsPath := options.WSPath
	if wsPath == "" {
		wsPath = "/ws"
	}

	fileServer := http.FileServer(http.FS(publicFS))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, apiPrefix) || strings.HasPrefix(req.URL.Path, wsPath) {
			http.NotFound(w, req)
			return
		}

		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		file, err := publicFS.Open(path)
		if err != nil {
			indexFile, indexErr := publicFS.Open("index.html")
			if indexErr != nil {
				http.NotFound(w, req)
				return
			}
			defer func() { _ = indexFile.Close() }()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.Copy(w, indexFile)
			return
		}
		_ = file.Close()
		fileServer.ServeHTTP(w, req)
	}))
	return true
}
