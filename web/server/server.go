// Package server is the web server's HTTP side: the login page, the
// account and user-administration pages, the Files page and API for the
// documents users keep on the server (files.go), and the pages that run
// the WebAssembly client (/word and /excel), whose files it serves only
// to logged-in users. Every response carries strict security headers, and
// every form post must come from this site's own pages.
package server

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"officelite/web"
	"officelite/web/auth"
	"officelite/web/files"
	"officelite/web/names"
	"officelite/web/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Config is what the server needs.
type Config struct {
	Auth *auth.Service
	// Files holds the users' documents.
	Files *files.Store
	// ClientDir holds the WebAssembly client: officelite.wasm (and
	// optionally .br / .gz copies) and wasm_exec.js.
	ClientDir string
	// InsecureCookies drops the Secure flag from the session cookie, for
	// plain-HTTP testing on a host other than localhost. Never in
	// production.
	InsecureCookies bool
	Log             *log.Logger
}

// Server serves the site.
type Server struct {
	cfg   Config
	pages map[string]*template.Template
	mux   *http.ServeMux
}

// pageNames are the templates rendered inside layout.html.
var pageNames = []string{"login", "home", "account", "users", "files"}

// New builds the server's routes and templates.
func New(cfg Config) (*Server, error) {
	if cfg.Log == nil {
		cfg.Log = log.Default()
	}
	s := &Server{cfg: cfg, pages: map[string]*template.Template{}, mux: http.NewServeMux()}
	funcs := template.FuncMap{
		"bytes": formatBytes,
		// For static/idle.js and static/serverfs.js.
		"idleSeconds":  func() int { return int(web.SessionIdle.Seconds()) },
		"warnSeconds":  func() int { return int(web.SessionIdleWarning.Seconds()) },
		"pingSeconds":  func() int { return int(web.SessionKeepAliveEvery.Seconds()) },
		"maxFileName":  func() int { return web.FileNameMaxLength },
		"storableExts": func() string { return strings.Join(names.StorableExts, " ") },
	}
	for _, name := range pageNames {
		t, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		s.pages[name] = t
	}
	app, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/app.html")
	if err != nil {
		return nil, err
	}
	s.pages["app"] = app
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}

	m := s.mux
	m.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	m.HandleFunc("GET /login", s.loginPage)
	m.HandleFunc("POST /login", s.login)
	m.HandleFunc("POST /logout", s.logout)

	m.HandleFunc("GET /{$}", s.user(s.home))
	m.HandleFunc("GET /word", s.user(s.app("word")))
	m.HandleFunc("GET /excel", s.user(s.app("excel")))
	m.HandleFunc("GET /client/{file}", s.user(s.clientFile))
	m.HandleFunc("GET /files", s.user(s.filesPage))
	m.HandleFunc("POST /files/upload", s.user(s.uploadFiles))
	m.HandleFunc("POST /files/rename", s.user(s.renameFile))
	m.HandleFunc("POST /files/delete", s.user(s.deleteFile))
	m.HandleFunc("GET /api/ping", s.user(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	m.HandleFunc("GET /api/files", s.user(s.apiList))
	m.HandleFunc("GET /api/files/{name}", s.user(s.apiGet))
	m.HandleFunc("PUT /api/files/{name}", s.user(s.apiPut))
	m.HandleFunc("POST /api/files/{name}/rename", s.user(s.apiRename))
	m.HandleFunc("DELETE /api/files/{name}", s.user(s.apiDelete))
	m.HandleFunc("GET /account", s.user(s.accountPage))
	m.HandleFunc("POST /account/password", s.user(s.changePassword))

	m.HandleFunc("GET /admin/users", s.admin(s.usersPage))
	m.HandleFunc("POST /admin/users", s.admin(s.createUser))
	m.HandleFunc("POST /admin/users/{id}/password", s.admin(s.resetPassword))
	m.HandleFunc("POST /admin/users/{id}/admin", s.admin(s.setAdmin))
	m.HandleFunc("POST /admin/users/{id}/delete", s.admin(s.deleteUser))
	return s, nil
}

// ServeHTTP adds the security headers, refuses cross-site form posts and
// routes the request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; "+
		"style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; "+
		"base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// sameOrigin reports whether a state-changing request came from this
// site's own pages, by the browser's Sec-Fetch-Site header or, from older
// browsers, the Origin header. Requests with neither (curl, tests) pass:
// the SameSite session cookie already keeps other sites' posts from
// being logged in.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
	default:
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

// ---- sessions and access ----

// cookieName is the session cookie's name. The __Host- prefix makes the
// browser refuse it unless it is Secure, for "/" and without a Domain, so
// no other joshco.com host can set or read it.
func (s *Server) cookieName() string {
	if s.cfg.InsecureCookies {
		return "officeweb_session"
	}
	return "__Host-officeweb_session"
}

// setSessionCookie sends the session cookie, or deletes it when token is
// empty.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	c := &http.Cookie{
		Name:     s.cookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
	}
	if token == "" {
		c.MaxAge = -1
	} else {
		c.Expires = expires
	}
	http.SetCookie(w, c)
}

// sessionToken returns the request's session token, or "".
func (s *Server) sessionToken(r *http.Request) string {
	c, err := r.Cookie(s.cookieName())
	if err != nil {
		return ""
	}
	return c.Value
}

// ctxKey keys the logged-in user in a request's context.
type ctxKey struct{}

// currentUser returns the logged-in user that user() put in the context.
func currentUser(r *http.Request) store.User {
	u, _ := r.Context().Value(ctxKey{}).(store.User)
	return u
}

// user wraps a handler for logged-in users only: others go to the login
// page (and come back afterwards), except that the client's files and the
// API answer 401, which a script can act on.
func (s *Server) user(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := s.sessionToken(r)
		sess, err := s.cfg.Auth.Authenticate(token)
		if err != nil {
			if !errors.Is(err, auth.ErrNoSession) {
				s.serverError(w, err)
				return
			}
			if token != "" {
				s.setSessionCookie(w, "", time.Time{})
			}
			if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/client/") || strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "not logged in", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		s.setSessionCookie(w, token, sess.ExpiresAt)
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess.User)))
	}
}

// admin wraps a handler for logged-in administrators only.
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return s.user(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsAdmin {
			http.Error(w, "administrators only", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// clientIP is the address the request came from. X-Forwarded-For is
// believed only from a proxy on this machine (Apache), and only its last
// entry, the one the proxy added: earlier ones are whatever the client
// sent.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	return host
}

// ---- rendering ----

// page is the data every template gets.
type page struct {
	Title string
	Nav   string      // the top bar's current section: "home", "word", "excel", "files", "account" or "users"
	User  *store.User // nil on the login page
	Msg   string      // a success message
	Err   string      // an error message
	Data  any         // the page's own data
}

// render executes a page template; status is the HTTP status.
func (s *Server) render(w http.ResponseWriter, status int, name string, p page) {
	t := s.pages[name]
	if p.Nav == "" {
		p.Nav = name
	}
	entry := "layout"
	if name == "app" {
		entry = "app"
	}
	var buf strings.Builder
	if err := t.ExecuteTemplate(&buf, entry, p); err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(buf.String())); err != nil {
		s.cfg.Log.Printf("write response: %v", err)
	}
}

// serverError logs err and answers 500 without details.
func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.cfg.Log.Printf("error: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// formatBytes shows a byte count in binary units (1 GiB, 512 MiB, …).
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return itoa(n) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	whole := n / div
	tenth := (n % div) * 10 / div
	s := itoa(whole)
	if tenth != 0 {
		s += "." + itoa(tenth)
	}
	return s + " " + string("KMGTPE"[exp]) + "iB"
}

// itoa formats n in decimal.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }
