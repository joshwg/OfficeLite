// Server storage for documents: the Files page (upload, download, rename,
// delete, the quota) and the JSON API the WebAssembly client's file
// system (static/serverfs.js) uses to open and save documents in place.
// Each user sees only their own folder (web/files); every name follows
// the rules in web/names.
//
//	GET    /api/files               {"used", "quota", "files": [{"name", "size", "modified"}]}
//	GET    /api/files/{name}        the document (as an attachment)
//	PUT    /api/files/{name}        store the body (?replace=0: only if new)
//	POST   /api/files/{name}/rename {"to": "new name"}
//	DELETE /api/files/{name}
//
// API errors are {"error": "message"} with 400 (name breaks a rule), 404,
// 409 (name taken), 413 (file over web.FileMaxBytes) or 507 (quota).

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
	case errors.Is(err, files.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, files.ErrExists):
		return http.StatusConflict
	case errors.Is(err, files.ErrTooLarge), errors.As(err, &mbe):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, files.ErrQuota):
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

// apiError sends a file store error as {"error": message}.
func (s *Server) apiError(w http.ResponseWriter, err error) {
	s.writeJSON(w, fileStatus(err), map[string]string{"error": s.fileMessage(err)})
}

// listing is GET /api/files's answer.
type listing struct {
	Used  int64        `json:"used"`
	Quota int64        `json:"quota"`
	Files []files.Info `json:"files"`
}

// apiList lists the user's documents.
func (s *Server) apiList(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	docs, used, err := s.cfg.Files.List(u.Username)
	if err != nil {
		s.apiError(w, err)
		return
	}
	if docs == nil {
		docs = []files.Info{} // [] rather than null
	}
	s.writeJSON(w, http.StatusOK, listing{Used: used, Quota: u.QuotaBytes, Files: docs})
}

// apiGet sends one of the user's documents, always as an attachment so a
// browser never shows a stored file as a page of this site.
func (s *Server) apiGet(w http.ResponseWriter, r *http.Request) {
	f, info, err := s.cfg.Files.Open(currentUser(r).Username, r.PathValue("name"))
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
	info, err := s.cfg.Files.Put(u.Username, r.PathValue("name"), r.Body, u.QuotaBytes, replace)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, info)
}

// apiRename renames one of the user's documents to the posted {"to"}.
func (s *Server) apiRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, web.HTTPFormMaxBytes)).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	info, err := s.cfg.Files.Rename(currentUser(r).Username, r.PathValue("name"), req.To)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, info)
}

// apiDelete deletes one of the user's documents.
func (s *Server) apiDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Files.Delete(currentUser(r).Username, r.PathValue("name")); err != nil {
		s.apiError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- the Files page ----

// fileRow is one document on the Files page.
type fileRow struct {
	files.Info
	App  string // "word" or "excel": the app that opens it, "" for none
	Href string // escaped name for URLs
}

// filesData is the Files page's data.
type filesData struct {
	Files   []fileRow
	Used    int64
	Quota   int64
	Percent int // of the quota used, 0–100
	MaxFile int64
	MaxName int
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

// filesPage lists the user's documents.
func (s *Server) filesPage(w http.ResponseWriter, r *http.Request) {
	s.renderFiles(w, r, http.StatusOK, message(r), "")
}

// renderFiles shows the Files page with a message or an error.
func (s *Server) renderFiles(w http.ResponseWriter, r *http.Request, status int, msg, errMsg string) {
	u := currentUser(r)
	docs, used, err := s.cfg.Files.List(u.Username)
	var ne *files.NameError
	if errors.As(err, &ne) {
		errMsg = noStorage
	} else if err != nil {
		s.serverError(w, err)
		return
	}
	data := filesData{Used: used, Quota: u.QuotaBytes, MaxFile: web.FileMaxBytes, MaxName: web.FileNameMaxLength}
	if u.QuotaBytes > 0 {
		data.Percent = int(min(100, used*100/u.QuotaBytes))
	}
	for _, d := range docs {
		data.Files = append(data.Files, fileRow{Info: d, App: appFor(d.Name), Href: url.PathEscape(d.Name)})
	}
	s.render(w, status, "files", page{Title: "Files", User: &u, Msg: msg, Err: errMsg, Data: data})
}

// uploadFiles stores the files of a multipart upload form, replacing
// ones of the same name only when its "replace" box (which comes before
// the files) is ticked. Every file is tried; the errors are listed.
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
	replace, stored := false, 0
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
		case part.FormName() == "replace":
			replace = true
		case part.FormName() == "file" && part.FileName() != "":
			name := part.FileName()
			if _, err := s.cfg.Files.Put(u.Username, name, part, u.QuotaBytes, replace); err != nil {
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
		s.renderFiles(w, r, http.StatusBadRequest, msg, strings.Join(problems, " · "))
	case stored == 0:
		s.renderFiles(w, r, http.StatusBadRequest, "", "Choose one or more files to upload.")
	default:
		s.cfg.Log.Printf("%s uploaded %d file(s)", u.Username, stored)
		http.Redirect(w, r, "/files?msg=uploaded", http.StatusSeeOther)
	}
}

// renameFile renames one of the user's documents from the Files page.
func (s *Server) renameFile(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, err := s.cfg.Files.Rename(currentUser(r).Username, r.PostForm.Get("name"), r.PostForm.Get("to")); err != nil {
		s.renderFiles(w, r, fileStatus(err), "", s.fileMessage(err))
		return
	}
	http.Redirect(w, r, "/files?msg=renamed", http.StatusSeeOther)
}

// deleteFile deletes one of the user's documents from the Files page,
// once the user said Yes to the page's question (confirm=1).
func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if r.PostForm.Get("confirm") == "" {
		s.renderFiles(w, r, http.StatusBadRequest, "", "Confirm the deletion: say Yes when asked (the page needs JavaScript).")
		return
	}
	if err := s.cfg.Files.Delete(currentUser(r).Username, r.PostForm.Get("name")); err != nil {
		s.renderFiles(w, r, fileStatus(err), "", s.fileMessage(err))
		return
	}
	http.Redirect(w, r, "/files?msg=filedeleted", http.StatusSeeOther)
}
