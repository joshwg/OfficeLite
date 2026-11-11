// The WebAssembly client: the /word and /excel pages that start it, and
// its files, served from Config.ClientDir to logged-in users only. A
// precompressed copy (officelite.wasm.br or .gz) is sent to browsers that
// accept it, which cuts the download several times.

package server

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// clientFiles are the files /client/ serves, with their content types.
var clientFiles = map[string]string{
	"officelite.wasm": "application/wasm",
	"wasm_exec.js":    "text/javascript; charset=utf-8",
}

// encodings are the precompressed copies looked for, best first.
var encodings = []struct{ name, ext string }{{"br", ".br"}, {"gzip", ".gz"}}

// appData is the app page's data.
type appData struct {
	App string // "word" or "excel": the app the client starts in
}

// app returns the handler for the page that runs the client as kind.
func (s *Server) app(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := "WordLite"
		if kind == "excel" {
			title = "ExcelLite"
		}
		s.render(w, http.StatusOK, "app", page{Title: title, Data: appData{App: kind}})
	}
}

// clientFile serves one of the client's files, precompressed when the
// browser accepts it and a compressed copy exists.
func (s *Server) clientFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	ctype, ok := clientFiles[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.ClientDir, name)
	accept := r.Header.Get("Accept-Encoding")
	w.Header().Add("Vary", "Accept-Encoding")
	for _, enc := range encodings {
		if acceptsEncoding(accept, enc.name) {
			if f, err := os.Open(path + enc.ext); err == nil {
				w.Header().Set("Content-Encoding", enc.name)
				s.serveFile(w, r, f, ctype)
				return
			}
		}
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		s.cfg.Log.Printf("client file %s missing: build it with ./web.sh build", path)
		http.Error(w, "the web client isn't built on this server", http.StatusNotFound)
		return
	} else if err != nil {
		s.serverError(w, err)
		return
	}
	s.serveFile(w, r, f, ctype)
}

// serveFile sends f (and closes it) with the given content type. Browsers
// keep it but check with the server (by its modification time) before
// using it again, so a new build is picked up at once.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, f *os.File, ctype string) {
	defer func() {
		if err := f.Close(); err != nil {
			s.cfg.Log.Printf("close %s: %v", f.Name(), err)
		}
	}()
	st, err := f.Stat()
	if err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// acceptsEncoding reports whether an Accept-Encoding header allows enc
// (and doesn't give it q=0).
func acceptsEncoding(header, enc string) bool {
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), enc) {
			continue
		}
		if _, p, err := mime.ParseMediaType("x/x;" + params); err == nil && strings.TrimSpace(params) != "" {
			if q := p["q"]; q == "0" || q == "0.0" || q == "0.00" || q == "0.000" {
				return false
			}
		}
		return true
	}
	return false
}
