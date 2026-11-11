// Tests of the HTTP side through httptest: login, access control, the
// admin pages, cross-site refusal and the client files. The Files page
// and API are tested in files_test.go.

package server

import (
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/web/auth"
	"officelite/web/files"
	"officelite/web/store"
)

// testPassword passes auth.ValidatePassword.
const testPassword = "correct horse battery"

// fixture is a running test server with an administrator "root" and a
// user "ann".
type fixture struct {
	srv    *httptest.Server
	svc    *auth.Service
	files  *files.Store
	client string // the client directory
}

// newFixture starts a test server.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := &auth.Service{Store: st}
	if _, err := svc.CreateUser("root", testPassword, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateUser("ann", testPassword, false); err != nil {
		t.Fatal(err)
	}
	clientDir := filepath.Join(dir, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	docs := files.New(filepath.Join(dir, "users"))
	h, err := New(Config{Auth: svc, Files: docs, ClientDir: clientDir, Log: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return &fixture{srv: srv, svc: svc, files: docs, client: clientDir}
}

// browser returns an HTTP client with a cookie jar that doesn't follow
// redirects, as a fresh browser.
func (f *fixture) browser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := f.srv.Client()
	c.Jar = jar
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// login logs c in as user and fails the test unless it worked.
func (f *fixture) login(t *testing.T, c *http.Client, user string) {
	t.Helper()
	resp := f.post(t, c, "/login", url.Values{"username": {user}, "password": {testPassword}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %s: status %d", user, resp.StatusCode)
	}
}

// get fetches path and returns the response with its body read.
func (f *fixture) get(t *testing.T, c *http.Client, path string) (*http.Response, string) {
	t.Helper()
	resp, err := c.Get(f.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

// post submits a form to path from this site's own pages.
func (f *fixture) post(t *testing.T, c *http.Client, path string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

// TestLoginRequired checks that pages and client files need a login, and
// that a bad password doesn't give one.
func TestLoginRequired(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	resp, _ := f.get(t, c, "/excel")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/login?next=") {
		t.Fatalf("GET /excel logged out: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := f.get(t, c, "/client/officelite.wasm"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("client file logged out: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/login", url.Values{"username": {"ann"}, "password": {"wrong password!"}}); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad password: %d", resp.StatusCode)
	}
	if resp, _ := f.get(t, c, "/"); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("home after bad password: %d", resp.StatusCode)
	}
	f.login(t, c, "ann")
	resp, body := f.get(t, c, "/excel")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `data-app="excel"`) {
		t.Fatalf("GET /excel logged in: %d", resp.StatusCode)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "'wasm-unsafe-eval'") {
		t.Fatalf("CSP = %q", csp)
	}
	f.post(t, c, "/logout", nil)
	if resp, _ := f.get(t, c, "/"); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("home after logout: %d", resp.StatusCode)
	}
}

// TestPasswordEye checks that the pages load the script that adds the
// show/hide eye to password fields, and that it is served.
func TestPasswordEye(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	if _, body := f.get(t, c, "/login"); !strings.Contains(body, `<script src="/static/password.js" defer>`) {
		t.Fatal("login page doesn't load password.js")
	}
	resp, body := f.get(t, c, "/static/password.js")
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") || !strings.Contains(body, "addEye") {
		t.Fatalf("password.js: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

// TestCookie checks the session cookie's security attributes.
func TestCookie(t *testing.T) {
	f := newFixture(t)
	resp := f.post(t, f.browser(t), "/login", url.Values{"username": {"ann"}, "password": {testPassword}})
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	c := cookies[0]
	if c.Name != "__Host-officeweb_session" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
		t.Fatalf("cookie = %+v", c)
	}
}

// TestAdminOnly checks that a plain user can't reach the user pages and
// an administrator can add a user.
func TestAdminOnly(t *testing.T) {
	f := newFixture(t)
	ann := f.browser(t)
	f.login(t, ann, "ann")
	if resp, _ := f.get(t, ann, "/admin/users"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("user list as ann: %d", resp.StatusCode)
	}
	if resp := f.post(t, ann, "/admin/users", url.Values{"username": {"eve"}, "password": {testPassword}}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("create as ann: %d", resp.StatusCode)
	}

	root := f.browser(t)
	f.login(t, root, "root")
	resp := f.post(t, root, "/admin/users", url.Values{"username": {"bob"}, "password": {testPassword}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create as root: %d", resp.StatusCode)
	}
	if _, body := f.get(t, root, "/admin/users"); !strings.Contains(body, "bob") {
		t.Fatal("bob not listed")
	}
	if resp := f.post(t, root, "/admin/users", url.Values{"username": {"BOB"}, "password": {testPassword}}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate create: %d", resp.StatusCode)
	}
	bob := f.browser(t)
	f.login(t, bob, "bob")
}

// TestAdminSelf checks that an administrator can't delete their own
// account from the user list.
func TestAdminSelf(t *testing.T) {
	f := newFixture(t)
	root := f.browser(t)
	f.login(t, root, "root")
	u, _, err := f.svc.Store.UserByName("root")
	if err != nil {
		t.Fatal(err)
	}
	path := "/admin/users/" + itoa(u.ID) + "/delete"
	if resp := f.post(t, root, path, url.Values{"confirm": {"1"}}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete self: %d", resp.StatusCode)
	}
}

// TestCrossSite checks that a post from another site is refused even
// with a valid session.
func TestCrossSite(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	f.login(t, c, "root")
	for _, hdr := range []struct{ k, v string }{{"Sec-Fetch-Site", "cross-site"}, {"Origin", "https://evil.example"}} {
		req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/admin/users",
			strings.NewReader(url.Values{"username": {"eve"}, "password": {testPassword}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(hdr.k, hdr.v)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s: %s → %d, want 403", hdr.k, hdr.v, resp.StatusCode)
		}
	}
	if _, _, err := f.svc.Store.UserByName("eve"); err == nil {
		t.Fatal("cross-site post created a user")
	}
}

// TestClientFiles checks the precompressed copy is chosen when accepted,
// and unknown names are refused.
func TestClientFiles(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.client, "officelite.wasm"), []byte("\x00asm plain"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.client, "officelite.wasm.br"), []byte("brotli"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := f.browser(t)
	f.login(t, c, "ann")

	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/client/officelite.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip, br")
	// A Transport sending its own Accept-Encoding would hide ours; with it
	// set by hand, the body arrives as stored.
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "br" || string(body) != "brotli" || resp.Header.Get("Content-Type") != "application/wasm" {
		t.Fatalf("br request: %v %q", resp.Header, body)
	}

	req.Header.Set("Accept-Encoding", "identity")
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "" || string(body) != "\x00asm plain" {
		t.Fatalf("identity request: %v %q", resp.Header, body)
	}

	if resp, _ := f.get(t, c, "/client/..%2ftest.db"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown client file: %d", resp.StatusCode)
	}
}

// TestHelpers checks safeNext, clientIP, acceptsEncoding and formatBytes.
func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{"/excel": "/excel", "//evil.com": "/", "/\\evil.com": "/", "https://evil.com": "/", "": "/"} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Errorf("clientIP via proxy = %q", got)
	}
	r.RemoteAddr = "198.51.100.7:5000"
	if got := clientIP(r); got != "198.51.100.7" {
		t.Errorf("clientIP direct with spoofed header = %q", got)
	}
	if !acceptsEncoding("gzip, br;q=0.8", "br") || acceptsEncoding("gzip, br;q=0", "br") || acceptsEncoding("gzip", "br") {
		t.Error("acceptsEncoding wrong")
	}
	for n, want := range map[int64]string{512: "512 B", 1 << 30: "1 GiB", 1536 << 20: "1.5 GiB"} {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
