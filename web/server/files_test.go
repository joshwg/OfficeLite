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

	"officelite/web"
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
	if status, body := f.call(t, c, http.MethodPost, "/api/move", `{"from":"Budget #1?.xlsx","to":" Plan.xlsx "}`); status != http.StatusOK || !strings.Contains(body, `"Plan.xlsx"`) {
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
	if l, _ := f.files.List("ann", ""); len(l.Files) != 0 {
		t.Fatalf("left: %+v", l)
	}
}

// TestAPIFolders checks folders through the API: making one (and the
// name rules), saving into it, listing it, moving a document and a
// folder, deleting only an empty folder unless all=1, and the depth
// limit.
func TestAPIFolders(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	f.login(t, c, "ann")
	if status, body := f.call(t, c, http.MethodPut, "/api/folders/Reports", ""); status != http.StatusOK || !strings.Contains(body, `"path":"Reports"`) {
		t.Fatalf("make folder: %d %s", status, body)
	}
	if status, _ := f.call(t, c, http.MethodPut, "/api/folders/reports", ""); status != http.StatusConflict {
		t.Fatalf("make it again in another case: %d", status)
	}
	for _, bad := range []string{".hidden", "Old.docx", "Reports/.x", "a%2Ab"} {
		if status, _ := f.call(t, c, http.MethodPut, "/api/folders/"+bad, ""); status != http.StatusBadRequest {
			t.Errorf("make folder %s: %d", bad, status)
		}
	}
	if status, _ := f.call(t, c, http.MethodPut, "/api/files/Missing/a.docx", "x"); status != http.StatusNotFound {
		t.Fatalf("save into a missing folder: %d", status)
	}
	if status, body := f.call(t, c, http.MethodPut, "/api/files/reports/Q%231.docx", "doc"); status != http.StatusOK || !strings.Contains(body, `"path":"Reports/Q#1.docx"`) {
		t.Fatalf("save into the folder: %d %s", status, body)
	}
	if l := f.list(t, c); len(l.Folders) != 1 || l.Folders[0].Name != "Reports" || len(l.Files) != 0 || l.Used != 3 {
		t.Fatalf("top listing = %+v", l)
	}
	status, body := f.call(t, c, http.MethodGet, "/api/files?dir=REPORTS", "")
	if status != http.StatusOK || !strings.Contains(body, `"dir":"Reports"`) || !strings.Contains(body, `"Q#1.docx"`) {
		t.Fatalf("folder listing: %d %s", status, body)
	}
	if status, _ := f.call(t, c, http.MethodGet, "/api/files?dir=Nope", ""); status != http.StatusNotFound {
		t.Fatalf("missing folder listing: %d", status)
	}
	if resp, got := f.get(t, c, "/api/files/Reports/Q%231.docx"); resp.StatusCode != http.StatusOK || got != "doc" {
		t.Fatalf("read from the folder: %d %q", resp.StatusCode, got)
	}
	if status, body := f.call(t, c, http.MethodDelete, "/api/folders/Reports", ""); status != http.StatusConflict || !strings.Contains(body, "ENOTEMPTY") {
		t.Fatalf("delete a full folder: %d %s", status, body)
	}
	if status, body := f.call(t, c, http.MethodPost, "/api/move", `{"from":"Reports/Q#1.docx","to":"Q#1.docx"}`); status != http.StatusOK || !strings.Contains(body, `"path":"Q#1.docx"`) {
		t.Fatalf("move to the top: %d %s", status, body)
	}
	if status, body := f.call(t, c, http.MethodPost, "/api/move", `{"from":"Reports","to":"Archive"}`); status != http.StatusOK {
		t.Fatalf("rename the folder: %d %s", status, body)
	}
	if status, _ := f.call(t, c, http.MethodPost, "/api/move", `{"from":"Archive","to":"Archive/In"}`); status != http.StatusBadRequest {
		t.Fatalf("move a folder into itself: %d", status)
	}
	if status, _ := f.call(t, c, http.MethodDelete, "/api/folders/Archive", ""); status != http.StatusNoContent {
		t.Fatalf("delete an empty folder: %d", status)
	}
	path := ""
	for i := 1; i <= web.FolderMaxDepth; i++ {
		path += "/L" + strconv.Itoa(i)
		if status, body := f.call(t, c, http.MethodPut, "/api/folders"+path, ""); status != http.StatusOK {
			t.Fatalf("make %s: %d %s", path, status, body)
		}
	}
	if status, _ := f.call(t, c, http.MethodPut, "/api/folders"+path+"/Deeper", ""); status != http.StatusBadRequest {
		t.Fatalf("past the depth: %d", status)
	}
	if status, _ := f.call(t, c, http.MethodPut, "/api/files"+path+"/deep.docx", "x"); status != http.StatusOK {
		t.Fatalf("document at the deepest: %d", status)
	}
	if status, _ := f.call(t, c, http.MethodDelete, "/api/folders/L1?all=1", ""); status != http.StatusNoContent {
		t.Fatalf("delete a folder and all in it: %d", status)
	}
	if l := f.list(t, c); len(l.Folders) != 0 || l.Used != 3 {
		t.Fatalf("after deletes = %+v", l)
	}
}

// TestFilesPageFolders checks the Files page's folders: making one,
// showing it with its path, uploading into it, renaming and moving
// documents and folders, and deleting a folder with what it holds.
func TestFilesPageFolders(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	f.login(t, c, "ann")
	if resp := f.post(t, c, "/files/mkdir", url.Values{"dir": {""}, "name": {"Reports"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("make folder: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/files/mkdir", url.Values{"dir": {""}, "name": {"Bad.docx"}}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("folder with a file's name: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/files/mkdir", url.Values{"dir": {"Reports"}, "name": {"2026"}}); resp.StatusCode != http.StatusSeeOther ||
		resp.Header.Get("Location") != "/files?dir=Reports&msg=folder" {
		t.Fatalf("make subfolder: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("dir", "Reports")
	fw, _ := mw.CreateFormFile("file", "Plan.docx")
	_, _ = fw.Write([]byte("plan"))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/files/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("upload into the folder: %d", resp.StatusCode)
	}
	if _, err := f.files.Stat("ann", "Reports/Plan.docx"); err != nil {
		t.Fatalf("uploaded file: %v", err)
	}
	resp, page := f.get(t, c, "/files?dir=Reports")
	if resp.StatusCode != http.StatusOK || !strings.Contains(page, ">Cloud files</a>") || !strings.Contains(page, "<strong>Reports</strong>") ||
		!strings.Contains(page, "/word?open=Reports%2fPlan.docx") || !strings.Contains(page, "/files?dir=Reports%2f2026") {
		t.Fatalf("folder page: %d\n%s", resp.StatusCode, page)
	}
	if resp := f.post(t, c, "/files/rename", url.Values{"name": {"Reports/Plan.docx"}, "to": {"Final.docx"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("rename in a folder: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/files/move", url.Values{"name": {"Reports/Final.docx"}, "dest": {"Reports/2026"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("move down: %d", resp.StatusCode)
	}
	if _, err := f.files.Stat("ann", "Reports/2026/Final.docx"); err != nil {
		t.Fatalf("after move: %v", err)
	}
	if resp := f.post(t, c, "/files/rename", url.Values{"name": {"Reports/2026"}, "to": {"Done"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("rename a folder: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/files/move", url.Values{"name": {"Reports/Done"}, "dest": {""}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("move a folder to the top: %d", resp.StatusCode)
	}
	if resp := f.post(t, c, "/files/delete", url.Values{"name": {"Done"}, "confirm": {"1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete a folder: %d", resp.StatusCode)
	}
	if l, _ := f.files.List("ann", ""); len(l.Folders) != 1 || l.Used != 0 {
		t.Fatalf("left: %+v", l)
	}
	if resp, _ := f.get(t, c, "/files?dir=Gone"); resp.StatusCode != http.StatusOK {
		t.Fatalf("missing folder page: %d", resp.StatusCode)
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

// TestFilesPageGone checks what the Files page does with something
// already gone (deleted from another tab, say): deleting it again counts
// as done, renaming or moving it says why it may be gone, and those
// failures, like a move into a missing folder, ask for a dialog box
// (data-alert).
func TestFilesPageGone(t *testing.T) {
	f := newFixture(t)
	c := f.browser(t)
	f.login(t, c, "ann")
	if _, err := f.files.Put("ann", "a.docx", strings.NewReader("x"), 100, false); err != nil {
		t.Fatal(err)
	}
	resp := f.post(t, c, "/files/delete", url.Values{"name": {"Gone.docx"}, "confirm": {"1"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/files?msg=gone" {
		t.Fatalf("delete of a gone file: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp := f.post(t, c, "/files/delete", url.Values{"name": {"Old"}, "confirm": {"1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete of a gone folder: %d", resp.StatusCode)
	}
	for _, tc := range []struct {
		path string
		form url.Values
		want []string
	}{
		{"/files/rename", url.Values{"name": {"Gone.docx"}, "to": {"New.docx"}},
			[]string{`data-alert="Couldn&#39;t rename"`, "“Gone.docx” is no longer there. It may have been deleted, moved or renamed in another tab."}},
		{"/files/move", url.Values{"name": {"a.docx"}, "dest": {"Nowhere"}},
			[]string{`data-alert="Couldn&#39;t move"`, "The folder “Nowhere” is no longer there."}},
		{"/files/mkdir", url.Values{"dir": {"Nowhere"}, "name": {"Sub"}},
			[]string{`data-alert="Couldn&#39;t make the folder"`, "The folder “Nowhere” is no longer there."}},
	} {
		status, body := f.postPage(t, c, tc.path, tc.form)
		if status != http.StatusNotFound {
			t.Errorf("%s: %d", tc.path, status)
		}
		for _, w := range tc.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: page lacks %q", tc.path, w)
			}
		}
	}
	resp2, page := f.get(t, c, "/files?dir=Gone")
	if resp2.StatusCode != http.StatusOK || !strings.Contains(page, `data-alert="Folder not found"`) {
		t.Fatalf("missing folder page: %d", resp2.StatusCode)
	}
	status, body := f.call(t, c, http.MethodDelete, "/api/files/Gone.docx", "")
	if status != http.StatusNotFound || !strings.Contains(body, "deleted, moved or renamed in another tab") {
		t.Fatalf("API delete of a gone file: %d %s", status, body)
	}
}

// postPage posts form to path like a page's form and returns the status
// and the page sent back.
func (f *fixture) postPage(t *testing.T, c *http.Client, path string, form url.Values) (int, string) {
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
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}
