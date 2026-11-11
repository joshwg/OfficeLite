// Tests of server storage over HTTP: the JSON API the client's file
// system uses, the Files page's forms, users' folders kept apart, and
// deleting a user's files with the account.

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"officelite/web/store"
)

// call sends a same-origin request with body and returns the status and
// the response body.
func (f *fixture) call(t *testing.T, c *http.Client, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

// list returns the user's listing from GET /api/files.
func (f *fixture) list(t *testing.T, c *http.Client) listing {
	t.Helper()
	status, body := f.call(t, c, http.MethodGet, "/api/files", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/files: %d %s", status, body)
	}
	var l listing
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

// TestAPI checks storing, listing, reading, renaming and deleting through
// the API, and its error statuses.
func TestAPI(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	if status, _ := f.call(t, c, http.MethodGet, "/api/files", ""); status != http.StatusUnauthorized {
		t.Fatalf("logged out: %d", status)
	}
	f.login(t, c, "ann")
	if l := f.list(t, c); len(l.Files) != 0 || l.Quota <= 0 {
		t.Fatalf("empty listing = %+v", l)
	}
	name := url.PathEscape("Budget #1?.xlsx")
	if status, body := f.call(t, c, http.MethodPut, "/api/files/"+name, "sheet"); status != http.StatusOK {
		t.Fatalf("PUT: %d %s", status, body)
	}
	if status, _ := f.call(t, c, http.MethodPut, "/api/files/budget%20%231%3F.XLSX?replace=0", "x"); status != http.StatusConflict {
		t.Fatalf("PUT existing with replace=0: %d", status)
	}
	l := f.list(t, c)
	if len(l.Files) != 1 || l.Files[0].Name != "Budget #1?.xlsx" || l.Used != 5 {
		t.Fatalf("listing = %+v", l)
	}
	resp, body := f.get(t, c, "/api/files/"+name)
	if resp.StatusCode != http.StatusOK || body != "sheet" || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("GET: %d %q %q", resp.StatusCode, body, resp.Header.Get("Content-Disposition"))
	}
	for _, bad := range []string{"..%2Fx.xlsx", "a%2Ab.xlsx", "caf%C3%A9.xlsx", "tab%09.xlsx", "x.exe"} {
		if status, _ := f.call(t, c, http.MethodPut, "/api/files/"+bad, "x"); status != http.StatusBadRequest {
			t.Errorf("PUT %s: %d", bad, status)
		}
	}
	if status, body := f.call(t, c, http.MethodPost, "/api/files/"+name+"/rename", `{"to":" Plan.xlsx "}`); status != http.StatusOK || !strings.Contains(body, `"Plan.xlsx"`) {
		t.Fatalf("rename: %d %s", status, body)
	}
	if status, _ := f.call(t, c, http.MethodGet, "/api/files/"+name, ""); status != http.StatusNotFound {
		t.Fatalf("old name after rename: %d", status)
	}
	if status, _ := f.call(t, c, http.MethodDelete, "/api/files/plan.XLSX", ""); status != http.StatusNoContent {
		t.Fatalf("DELETE: %d", status)
	}
	if l := f.list(t, c); len(l.Files) != 0 {
		t.Fatalf("after delete: %+v", l)
	}
}

// TestAPIQuota checks that a save past the quota is refused with 507.
func TestAPIQuota(t *testing.T) {
	f := newFixture(t)
	ann, _, err := f.svc.Store.UserByName("ann")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Store.SetQuota(ann.ID, 10); err != nil {
		t.Fatal(err)
	}
	c := f.browser(t)
	f.login(t, c, "ann")
	if status, _ := f.call(t, c, http.MethodPut, "/api/files/a.docx", "12345678901"); status != http.StatusInsufficientStorage {
		t.Fatalf("over quota: %d", status)
	}
	if status, _ := f.call(t, c, http.MethodPut, "/api/files/a.docx", "1234567890"); status != http.StatusOK {
		t.Fatalf("exactly the quota: %d", status)
	}
}

// TestUsersApart checks that a user sees and reaches only their own files.
func TestUsersApart(t *testing.T) {
	f := newFixture(t)
	if _, err := f.files.Put("root", "secret.docx", strings.NewReader("x"), 100, false); err != nil {
		t.Fatal(err)
	}
	c := f.browser(t)
	f.login(t, c, "ann")
	if l := f.list(t, c); len(l.Files) != 0 {
		t.Fatalf("ann sees %+v", l.Files)
	}
	if status, _ := f.call(t, c, http.MethodGet, "/api/files/secret.docx", ""); status != http.StatusNotFound {
		t.Fatalf("ann reads root's file: %d", status)
	}
}

// TestFilesPage checks the page, an upload (refused over an existing
// file unless Replace is ticked), a rename and a delete.
func TestFilesPage(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	f.login(t, c, "ann")
	upload := func(replace bool, name, content string) int {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		if replace {
			_ = mw.WriteField("replace", "1")
		}
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte(content))
		_ = mw.Close()
		req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/files/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if status := upload(false, "Report.docx", "one"); status != http.StatusSeeOther {
		t.Fatalf("upload: %d", status)
	}
	if status := upload(false, "report.docx", "two"); status != http.StatusBadRequest {
		t.Fatalf("upload over existing: %d", status)
	}
	if status := upload(true, "report.docx", "three"); status != http.StatusSeeOther {
		t.Fatalf("upload with replace: %d", status)
	}
	resp, page := f.get(t, c, "/files")
	if resp.StatusCode != http.StatusOK || !strings.Contains(page, "Report.docx") || !strings.Contains(page, "/word?open=Report.docx") {
		t.Fatalf("Files page: %d\n%s", resp.StatusCode, page)
	}
	if d, _ := f.files.Stat("ann", "Report.docx"); d.Size != 5 {
		t.Fatalf("after replace: %+v", d)
	}
	if resp := f.post(t, c, "/files/rename", url.Values{"name": {"Report.docx"}, "to": {"Final.docx"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("rename: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/files/delete", url.Values{"name": {"Final.docx"}}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete without confirm: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/files/delete", url.Values{"name": {"Final.docx"}, "confirm": {"1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if docs, _, _ := f.files.List("ann"); len(docs) != 0 {
		t.Fatalf("left: %+v", docs)
	}
}

// TestDeleteUserFiles checks that deleting a user deletes their folder,
// and that a leftover folder blocks a new account of the same name.
func TestDeleteUserFiles(t *testing.T) {
	f := newFixture(t)
	if _, err := f.files.Put("ann", "a.docx", strings.NewReader("x"), 100, false); err != nil {
		t.Fatal(err)
	}
	ann, _, err := f.svc.Store.UserByName("ann")
	if err != nil {
		t.Fatal(err)
	}
	c := f.browser(t)
	f.login(t, c, "root")
	resp := f.post(t, c, "/admin/users/"+strconv.FormatInt(ann.ID, 10)+"/delete", url.Values{"confirm": {"1"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete user: %d", resp.StatusCode)
	}
	if has, _ := f.files.HasFolder("ann"); has {
		t.Fatal("ann's folder left")
	}
	if _, err := f.svc.Store.UserByID(ann.ID); err != store.ErrNotFound {
		t.Fatalf("ann still there: %v", err)
	}

	// A folder left behind (here made by hand) blocks re-creating "Ann".
	dir, _ := f.files.Dir("ann")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	resp = f.post(t, c, "/admin/users", url.Values{"username": {"Ann"}, "password": {testPassword}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create over leftover folder: %d", resp.StatusCode)
	}
}
