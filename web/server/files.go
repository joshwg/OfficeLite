// Server storage for documents (cloud files): the Files page (folders,
// upload, download, rename, move, delete, the quota) and the JSON API the
// WebAssembly client's file system (static/serverfs.js) uses to open and
// save documents in place. Each user sees only their own folder
// (web/files); every name and path follows the rules in web/names.
// Paths join folder names and a file name with '/', each part escaped:
// "Reports/2026/Budget%20%231.xlsx".
//
//	GET    /api/files?dir=PATH      {"used", "quota", "dir", "folders": [{"name", "path", "modified"}],
//	                                 "files": [{"name", "path", "size", "modified"}]}
//	GET    /api/files/{path}        the document (as an attachment)
//	PUT    /api/files/{path}        store the body (?replace=0: only if new)
//	DELETE /api/files/{path}
//	PUT    /api/folders/{path}      make the folder
//	DELETE /api/folders/{path}      delete the folder (?all=1: with all in it, else only if empty)
//	POST   /api/move                {"from": path, "to": path}: move or rename a document or folder
//	GET    /api/recent              the apps' recent-files list (a JSON array)
//	PUT    /api/recent              store the body as that list
//
// API errors are {"error": "message", "code": errno} with 400 (name
// breaks a rule), 404, 409 (name taken, or a folder isn't empty: code
// ENOTEMPTY), 413 (file over web.FileMaxBytes) or 507 (quota, or too
// many folders).

package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"officelite/web"
	"officelite/web/files"
)

// fileStatus is the HTTP status of a file store error.
func fileStatus(err error) int {
	var ne *files.NameError
	var mbe *http.MaxBytesError
	switch {
	case errors.As(err, &ne):
		return http.StatusBadRequest
	case errors.Is(err, files.ErrTooDeep), errors.Is(err, files.ErrIntoItself):
		return http.StatusBadRequest
	case errors.Is(err, files.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, files.ErrExists), errors.Is(err, files.ErrNotEmpty):
		return http.StatusConflict
	case errors.Is(err, files.ErrTooLarge), errors.As(err, &mbe):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, files.ErrQuota), errors.Is(err, files.ErrTooManyFolders):
		return http.StatusInsufficientStorage
	default:
		return http.StatusInternalServerError
	}
}

// fileMessage is what the user is told about a file store error: the
// error itself for expected ones, nothing more for the rest (logged).
func (s *Server) fileMessage(err error) string {
	if fileStatus(err) == http.StatusInternalServerError {
		s.cfg.Log.Printf("error: %v", err)
		return "The server couldn't do that; please try again."
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return files.ErrTooLarge.Error()
	}
	return capitalize(err.Error())
}

// capitalize upper-cases the first letter of a message.
func capitalize(msg string) string {
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}

// allowUpload gives the request FileUploadTimeout to arrive in place of
// the server's shorter HTTPReadTimeout.
func (s *Server) allowUpload(w http.ResponseWriter) {
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(web.FileUploadTimeout)); err != nil {
		s.cfg.Log.Printf("set upload deadline: %v", err)
	}
}

// ---- JSON API ----

// writeJSON sends v as JSON with status.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.cfg.Log.Printf("write response: %v", err)
	}
}

// apiError sends a file store error as {"error": message}, with the
// errno the client's file system should report where the status alone
// doesn't say it ("code").
func (s *Server) apiError(w http.ResponseWriter, err error) {
	body := map[string]string{"error": s.fileMessage(err)}
	if errors.Is(err, files.ErrNotEmpty) {
		body["code"] = "ENOTEMPTY"
	}
	s.writeJSON(w, fileStatus(err), body)
}

// listing is GET /api/files's answer.
type listing struct {
	files.Listing
	Quota int64 `json:"quota"`
}

// apiList lists one of the user's folders (?dir=, the top one when
// absent).
func (s *Server) apiList(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	l, err := s.cfg.Files.List(u.Username, r.URL.Query().Get("dir"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	if l.Folders == nil {
		l.Folders = []files.Folder{} // [] rather than null
	}
	if l.Files == nil {
		l.Files = []files.Info{}
	}
	s.writeJSON(w, http.StatusOK, listing{Listing: l, Quota: u.QuotaBytes})
}

// apiGet sends one of the user's documents, always as an attachment so a
// browser never shows a stored file as a page of this site.
func (s *Server) apiGet(w http.ResponseWriter, r *http.Request) {
	f, info, err := s.cfg.Files.Open(currentUser(r).Username, r.PathValue("path"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			s.cfg.Log.Printf("close %s: %v", f.Name(), err)
		}
	}()
	ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(info.Name)))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name}))
	h.Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", info.Modified, f)
}

// apiPut stores the request body as one of the user's documents,
// replacing one of the same name unless ?replace=0.
func (s *Server) apiPut(w http.ResponseWriter, r *http.Request) {
	s.allowUpload(w)
	u := currentUser(r)
	replace := r.URL.Query().Get("replace") != "0"
	info, err := s.cfg.Files.Put(u.Username, r.PathValue("path"), r.Body, u.QuotaBytes, replace)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, info)
}

// apiRecent sends the user's recent-files list (a JSON array, [] when
// there is none yet), which the apps keep here in the browser
// (common/recent).
func (s *Server) apiRecent(w http.ResponseWriter, r *http.Request) {
	data, err := s.cfg.Files.Recent(currentUser(r).Username)
	if err != nil {
		s.apiError(w, err)
		return
	}
	if data == nil {
		data = []byte("[]")
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	if _, err := w.Write(data); err != nil {
		s.cfg.Log.Printf("write response: %v", err)
	}
}

// apiSetRecent stores the request body, a JSON array of at most
// web.RecentMaxBytes, as the user's recent-files list.
func (s *Server) apiSetRecent(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, web.RecentMaxBytes+1))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Couldn't read the list."})
		return
	}
	if len(data) > web.RecentMaxBytes {
		s.writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "The recent-files list is too long."})
		return
	}
	var list []json.RawMessage
	if err := json.Unmarshal(data, &list); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The recent-files list isn't a JSON array."})
		return
	}
	if err := s.cfg.Files.SetRecent(currentUser(r).Username, data); err != nil {
		s.apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiMove moves or renames one of the user's documents or folders, as
// posted: {"from": path, "to": path}. It answers {"path"}: the new path
// as stored.
func (s *Server) apiMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, web.HTTPFormMaxBytes)).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	path, err := s.cfg.Files.Move(currentUser(r).Username, req.From, req.To)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// apiMakeFolder makes one of the user's folders.
func (s *Server) apiMakeFolder(w http.ResponseWriter, r *http.Request) {
	f, err := s.cfg.Files.MakeFolder(currentUser(r).Username, r.PathValue("path"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, f)
}

// apiDeleteFolder deletes one of the user's folders: only an empty one,
// unless ?all=1.
func (s *Server) apiDeleteFolder(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1"
	if err := s.cfg.Files.DeleteFolder(currentUser(r).Username, r.PathValue("path"), all); err != nil {
		s.apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiDelete deletes one of the user's documents.
func (s *Server) apiDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Files.Delete(currentUser(r).Username, r.PathValue("path")); err != nil {
		s.apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- the Files page ----

// crumb is one step of the Files page's folder path, from the top.
type crumb struct {
	Name string
	Dir  string // its path, for /files?dir=
}

// fileRow is one document or folder on the Files page.
type fileRow struct {
	Name     string
	Path     string // with its folders, as stored
	Folder   bool
	Size     int64
	Modified time.Time
	App      string // "word" or "excel": the app that opens a document, "" for none
	Href     string // the escaped path, for /api/files/
}

// filesData is the Files page's data.
type filesData struct {
	Dir      string  // the folder shown, as stored; "" for the top
	Crumbs   []crumb // the folders above it and itself, from the top
	Rows     []fileRow
	Folders  []string // all the user's folders, for moving to
	CanNest  bool     // whether Dir may hold more folders (web.FolderMaxDepth)
	Used     int64
	Quota    int64
	Percent  int // of the quota used, 0–100
	MaxFile  int64
	MaxName  int
	MaxDepth int
}

// appFor is the app that opens a document named name: "word", "excel",
// or "" for one neither opens (a PDF).
func appFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".docx", ".docm", ".dotx", ".dotm", ".doc", ".dot", ".pages", ".txt":
		return "word"
	case ".xlsx", ".xlsm", ".xls", ".csv", ".tsv", ".numbers":
		return "excel"
	}
	return ""
}

// escapePath escapes each part of a '/'-joined path for a URL path.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// parentOf is the folder holding path ("" for the top).
func parentOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

// joinDir joins a folder path and a name ("" is the top folder).
func joinDir(dir, name string) string {
	dir = strings.Trim(dir, " /")
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// filesURL is the Files page showing folder dir, with message code msg
// ("" for none).
func filesURL(dir, msg string) string {
	q := url.Values{}
	if dir != "" {
		q.Set("dir", dir)
	}
	if msg != "" {
		q.Set("msg", msg)
	}
	if len(q) == 0 {
		return "/files"
	}
	return "/files?" + q.Encode()
}

// filesPage lists one of the user's folders (?dir=, the top one when
// absent).
func (s *Server) filesPage(w http.ResponseWriter, r *http.Request) {
	s.renderFiles(w, r, http.StatusOK, r.URL.Query().Get("dir"), message(r), "")
}

// renderFiles shows the Files page at folder dir with a message or an
// error; a folder that is gone shows the top one.
func (s *Server) renderFiles(w http.ResponseWriter, r *http.Request, status int, dir, msg, errMsg string) {
	u := currentUser(r)
	l, err := s.cfg.Files.List(u.Username, dir)
	if dir != "" && (errors.Is(err, files.ErrNotFound) || fileStatus(err) == http.StatusBadRequest) {
		if errMsg == "" {
			errMsg = "That folder isn't there any more."
		}
		l, err = s.cfg.Files.List(u.Username, "")
	}
	var ne *files.NameError
	if errors.As(err, &ne) {
		errMsg = noStorage
	} else if err != nil {
		s.serverError(w, err)
		return
	}
	all, err := s.cfg.Files.AllFolders(u.Username)
	if err != nil && !errors.As(err, &ne) {
		s.serverError(w, err)
		return
	}
	depth := 0
	if l.Dir != "" {
		depth = strings.Count(l.Dir, "/") + 1
	}
	data := filesData{Dir: l.Dir, Folders: all, CanNest: depth < web.FolderMaxDepth, Used: l.Used, Quota: u.QuotaBytes,
		MaxFile: web.FileMaxBytes, MaxName: web.FileNameMaxLength, MaxDepth: web.FolderMaxDepth}
	if u.QuotaBytes > 0 {
		data.Percent = int(min(100, l.Used*100/u.QuotaBytes))
	}
	if l.Dir != "" {
		at := ""
		for _, part := range strings.Split(l.Dir, "/") {
			at = joinDir(at, part)
			data.Crumbs = append(data.Crumbs, crumb{Name: part, Dir: at})
		}
	}
	for _, f := range l.Folders {
		data.Rows = append(data.Rows, fileRow{Name: f.Name, Path: f.Path, Folder: true, Modified: f.Modified})
	}
	for _, d := range l.Files {
		data.Rows = append(data.Rows, fileRow{Name: d.Name, Path: d.Path, Size: d.Size, Modified: d.Modified,
			App: appFor(d.Name), Href: escapePath(d.Path)})
	}
	s.render(w, status, "files", page{Title: "Files", User: &u, Msg: msg, Err: errMsg, Data: data})
}

// uploadFiles stores the files of a multipart upload form in its folder
// (its "dir" field), replacing ones of the same name only when its
// "replace" box is ticked; both fields come before the files. Every file
// is tried; the errors are listed.
func (s *Server) uploadFiles(w http.ResponseWriter, r *http.Request) {
	s.allowUpload(w)
	u := currentUser(r)
	// The quota caps what can be stored; the slack covers the form's
	// own bytes around the files.
	r.Body = http.MaxBytesReader(w, r.Body, u.QuotaBytes+web.HTTPFormMaxBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	dir, replace, stored := "", false, 0
	var problems []string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			problems = append(problems, s.fileMessage(err))
			break
		}
		switch {
		case part.FormName() == "dir":
			b, err := io.ReadAll(io.LimitReader(part, web.HTTPFormMaxBytes))
			if err != nil {
				problems = append(problems, s.fileMessage(err))
			}
			dir = string(b)
		case part.FormName() == "replace":
			replace = true
		case part.FormName() == "file" && part.FileName() != "":
			name := part.FileName()
			if _, err := s.cfg.Files.Put(u.Username, joinDir(dir, name), part, u.QuotaBytes, replace); err != nil {
				msg := s.fileMessage(err)
				if errors.Is(err, files.ErrExists) {
					msg += " (tick Replace to overwrite it)"
				}
				problems = append(problems, strings.TrimSpace(name)+": "+msg)
			} else {
				stored++
			}
		}
		if err := part.Close(); err != nil {
			s.cfg.Log.Printf("close upload part: %v", err)
		}
	}
	switch {
	case len(problems) > 0:
		msg := ""
		if stored > 0 {
			msg = "Some files were uploaded."
		}
		s.renderFiles(w, r, http.StatusBadRequest, dir, msg, strings.Join(problems, " · "))
	case stored == 0:
		s.renderFiles(w, r, http.StatusBadRequest, dir, "", "Choose one or more files to upload.")
	default:
		s.cfg.Log.Printf("%s uploaded %d file(s)", u.Username, stored)
		http.Redirect(w, r, filesURL(dir, "uploaded"), http.StatusSeeOther)
	}
}

// makeFolder makes a folder named "name" in the Files page's folder
// "dir".
func (s *Server) makeFolder(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	dir := r.PostForm.Get("dir")
	if _, err := s.cfg.Files.MakeFolder(currentUser(r).Username, joinDir(dir, r.PostForm.Get("name"))); err != nil {
		s.renderFiles(w, r, fileStatus(err), dir, "", s.fileMessage(err))
		return
	}
	http.Redirect(w, r, filesURL(dir, "folder"), http.StatusSeeOther)
}

// renameFile renames one of the user's documents or folders ("name", a
// path) to "to" in the same folder, from the Files page.
func (s *Server) renameFile(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	from := r.PostForm.Get("name")
	dir := parentOf(from)
	if _, err := s.cfg.Files.Move(currentUser(r).Username, from, joinDir(dir, r.PostForm.Get("to"))); err != nil {
		s.renderFiles(w, r, fileStatus(err), dir, "", s.fileMessage(err))
		return
	}
	http.Redirect(w, r, filesURL(dir, "renamed"), http.StatusSeeOther)
}

// moveFile moves one of the user's documents or folders ("name", a path)
// into the folder "dest" ("" or "/" for the top), from the Files page.
func (s *Server) moveFile(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	from := r.PostForm.Get("name")
	dir := parentOf(from)
	base := from[strings.LastIndexByte(from, '/')+1:]
	if _, err := s.cfg.Files.Move(currentUser(r).Username, from, joinDir(r.PostForm.Get("dest"), base)); err != nil {
		s.renderFiles(w, r, fileStatus(err), dir, "", s.fileMessage(err))
		return
	}
	http.Redirect(w, r, filesURL(dir, "moved"), http.StatusSeeOther)
}

// deleteFile deletes one of the user's documents, or a folder with all
// in it, from the Files page, once the user said Yes to the page's
// question (confirm=1).
func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PostForm.Get("name")
	dir := parentOf(name)
	if r.PostForm.Get("confirm") == "" {
		s.renderFiles(w, r, http.StatusBadRequest, dir, "", "Confirm the deletion: say Yes when asked (the page needs JavaScript).")
		return
	}
	u := currentUser(r).Username
	var err error
	msg := "filedeleted"
	if files.IsFolderPath(name) {
		err, msg = s.cfg.Files.DeleteFolder(u, name, true), "folderdeleted"
	} else {
		err = s.cfg.Files.Delete(u, name)
	}
	if err != nil {
		s.renderFiles(w, r, fileStatus(err), dir, "", s.fileMessage(err))
		return
	}
	http.Redirect(w, r, filesURL(dir, msg), http.StatusSeeOther)
}
