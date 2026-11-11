// A test of static/serverfs.js with Go's real os package: it builds
// testdata/fsprobe for WebAssembly and runs it under Node against a test
// server. Skipped without Node or with -short (the build takes a while).

package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"officelite/web"
	"officelite/web/auth"
	"officelite/web/files"
	"officelite/web/names"
	"officelite/web/store"
)

// TestServerFS runs the probe and fails with its output unless it passed.
func TestServerFS(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a WebAssembly program")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	probe := filepath.Join(dir, "fsprobe.wasm")
	build := exec.Command("go", "build", "-o", probe, "./testdata/fsprobe")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fsprobe: %v\n%s", err, out)
	}

	// A plain-HTTP server (the probe's fetch can't check a test
	// certificate) with user ann, whose quota is 1 KiB.
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := &auth.Service{Store: st}
	ann, err := svc.CreateUser("ann", testPassword, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetQuota(ann.ID, 1024); err != nil {
		t.Fatal(err)
	}
	h, err := New(Config{Auth: svc, Files: files.New(filepath.Join(dir, "users")), ClientDir: dir,
		InsecureCookies: true, Log: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := srv.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.PostForm(srv.URL+"/login", url.Values{"username": {"ann"}, "password": {testPassword}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	var cookie string
	for _, ck := range resp.Cookies() {
		cookie = ck.Name + "=" + ck.Value
	}
	if cookie == "" {
		t.Fatalf("no session cookie (status %d)", resp.StatusCode)
	}

	run := exec.Command(node, "testdata/fsprobe/run.mjs", "static/serverfs.js",
		filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js"), probe)
	run.Env = append(os.Environ(), "BASE="+srv.URL, "COOKIE="+cookie,
		"MAX_NAME="+strconv.Itoa(web.FileNameMaxLength), "EXTS="+strings.Join(names.StorableExts, " "))
	out, err := run.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("fsprobe: %v\n%s", err, out)
	}
}
