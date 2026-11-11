// The page handlers: logging in and out, the home page, the user's own
// account page and the administrators' user list. Forms post back and
// redirect (with a message code in ?msg=) on success, or re-render with
// the error on failure.

package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"officelite/web"
	"officelite/web/auth"
	"officelite/web/files"
	"officelite/web/store"
)

// messages are the success messages a redirect's ?msg= code shows.
var messages = map[string]string{
	"created":  "User created.",
	"deleted":  "User deleted.",
	"password": "Password changed.",
	"admin":    "Administrator status changed.",
	// The Files page's.
	"uploaded":    "Uploaded.",
	"renamed":     "File renamed.",
	"filedeleted": "File deleted.",
}

// message returns the success message for the request's ?msg= code.
func message(r *http.Request) string { return messages[r.URL.Query().Get("msg")] }

// parseForm reads a form post of at most HTTPFormMaxBytes.
func parseForm(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, web.HTTPFormMaxBytes)
	return r.ParseForm()
}

// safeNext returns where to go after logging in: next when it is a path
// on this site, else "/". "//host" and "/\host" are refused, as browsers
// take them for other sites.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// ---- login ----

// loginData is the login page's data.
type loginData struct {
	Username string
	Next     string
	Wait     int // seconds until the username may try again (0: not throttled)
}

// loginPage shows the login form, or goes home when already logged in.
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, err := s.cfg.Auth.Authenticate(s.sessionToken(r)); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "login", page{Title: "Log in", Data: loginData{Next: r.URL.Query().Get("next")}})
}

// login checks the posted credentials and starts a session.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := r.PostForm.Get("username")
	data := loginData{Username: username, Next: r.PostForm.Get("next")}
	token, _, err := s.cfg.Auth.Login(username, r.PostForm.Get("password"), clientIP(r))
	var throttled *auth.ThrottledError
	switch {
	case errors.As(err, &throttled):
		s.cfg.Log.Printf("throttled login for %q from %s", username, clientIP(r))
		data.Wait = throttled.Seconds() // the page's countdown says it; a fixed message would go stale
		s.render(w, http.StatusTooManyRequests, "login", page{Title: "Log in", Data: data})
		return
	case errors.Is(err, auth.ErrBadCredentials):
		s.cfg.Log.Printf("failed login for %q from %s", username, clientIP(r))
		s.render(w, http.StatusUnauthorized, "login", page{Title: "Log in", Err: err.Error(), Data: data})
		return
	case err != nil:
		s.serverError(w, err)
		return
	}
	s.setSessionCookie(w, token, time.Now().Add(web.SessionIdle))
	http.Redirect(w, r, safeNext(data.Next), http.StatusSeeOther)
}

// logout ends the session.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Auth.Logout(s.sessionToken(r)); err != nil {
		s.serverError(w, err)
		return
	}
	s.setSessionCookie(w, "", time.Time{})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ---- home and account ----

// home offers WordLite and ExcelLite.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	s.render(w, http.StatusOK, "home", page{Title: "OfficeLite", User: &u})
}

// noStorage tells a user whose name breaks today's rules (an account
// made before them) why they have no server storage.
const noStorage = "Your username can't hold files on the server under today's rules; ask an administrator for an account with a new name."

// accountData is the account page's data.
type accountData struct {
	Used int64 // bytes of server storage in use
}

// accountPage shows the user's account and the change-password form.
func (s *Server) accountPage(w http.ResponseWriter, r *http.Request) {
	s.renderAccount(w, r, http.StatusOK, message(r), "")
}

// renderAccount shows the account page with a message or an error.
func (s *Server) renderAccount(w http.ResponseWriter, r *http.Request, status int, msg, errMsg string) {
	u := currentUser(r)
	_, used, err := s.cfg.Files.List(u.Username)
	var ne *files.NameError
	if errors.As(err, &ne) && errMsg == "" {
		errMsg = noStorage
	} else if err != nil && !errors.As(err, &ne) {
		s.serverError(w, err)
		return
	}
	s.render(w, status, "account", page{Title: "Account", User: &u, Msg: msg, Err: errMsg, Data: accountData{Used: used}})
}

// changePassword changes the user's own password. Every session of the
// user ends, so this browser gets a new one.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	f := r.PostForm
	fail := func(msg string) { s.renderAccount(w, r, http.StatusBadRequest, "", msg) }
	if f.Get("new") != f.Get("confirm") {
		fail("The new passwords don't match.")
		return
	}
	if err := s.cfg.Auth.ChangePassword(u.ID, f.Get("current"), f.Get("new")); err != nil {
		fail(err.Error())
		return
	}
	token, err := s.cfg.Auth.StartSession(u.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.setSessionCookie(w, token, time.Now().Add(web.SessionIdle))
	http.Redirect(w, r, "/account?msg=password", http.StatusSeeOther)
}

// ---- user administration ----

// userRow is one account on the user list.
type userRow struct {
	store.User
	Used int64 // bytes of server storage in use
}

// usersData is the user list page's data.
type usersData struct {
	Users []userRow
	// Form values to keep after a failed create.
	NewUsername string
	NewAdmin    bool
	MaxName     int // web.UsernameMaxLength, for the form
}

// usersPage lists the accounts.
func (s *Server) usersPage(w http.ResponseWriter, r *http.Request) {
	s.renderUsers(w, r, http.StatusOK, message(r), "", usersData{})
}

// renderUsers shows the user list with a message or an error.
func (s *Server) renderUsers(w http.ResponseWriter, r *http.Request, status int, msg, errMsg string, data usersData) {
	users, err := s.cfg.Auth.Store.Users()
	if err != nil {
		s.serverError(w, err)
		return
	}
	data.MaxName = web.UsernameMaxLength
	data.Users = data.Users[:0]
	for _, user := range users {
		_, used, err := s.cfg.Files.List(user.Username)
		if err != nil {
			// An account whose name breaks today's rules has no folder.
			s.cfg.Log.Printf("storage of user %q: %v", user.Username, err)
		}
		data.Users = append(data.Users, userRow{User: user, Used: used})
	}
	u := currentUser(r)
	s.render(w, status, "users", page{Title: "Users", User: &u, Msg: msg, Err: errMsg, Data: data})
}

// createUser adds an account.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	f := r.PostForm
	data := usersData{NewUsername: f.Get("username"), NewAdmin: f.Get("admin") != ""}
	if err := CheckNoFolder(s.cfg.Files, strings.TrimSpace(f.Get("username"))); err != nil {
		s.renderUsers(w, r, http.StatusBadRequest, "", err.Error(), data)
		return
	}
	created, err := s.cfg.Auth.CreateUser(f.Get("username"), f.Get("password"), data.NewAdmin)
	if err != nil {
		s.renderUsers(w, r, http.StatusBadRequest, "", err.Error(), data)
		return
	}
	s.cfg.Log.Printf("%s created user %q (admin %v)", currentUser(r).Username, created.Username, created.IsAdmin)
	http.Redirect(w, r, "/admin/users?msg=created", http.StatusSeeOther)
}

// targetUser reads the {id} of an admin action and refuses actions on
// the administrator's own account (which go through /account), so that
// nobody locks themselves out by accident.
func (s *Server) targetUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	if id == currentUser(r).ID {
		s.renderUsers(w, r, http.StatusBadRequest, "", "Use the Account page to change your own account.", usersData{})
		return 0, false
	}
	if err := parseForm(w, r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// adminResult finishes an admin action: a redirect with msg on success,
// the error on the user list otherwise.
func (s *Server) adminResult(w http.ResponseWriter, r *http.Request, err error, msg string) {
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/users?msg="+msg, http.StatusSeeOther)
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
	default:
		s.renderUsers(w, r, http.StatusBadRequest, "", err.Error(), usersData{})
	}
}

// resetPassword sets another user's password (ending their sessions).
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	err := s.cfg.Auth.SetPassword(id, r.PostForm.Get("password"))
	if err == nil {
		s.cfg.Log.Printf("%s reset the password of user %d", currentUser(r).Username, id)
	}
	s.adminResult(w, r, err, "password")
}

// setAdmin makes another user an administrator or not.
func (s *Server) setAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	admin := r.PostForm.Get("admin") == "1"
	err := s.cfg.Auth.Store.SetAdmin(id, admin)
	if err == nil {
		s.cfg.Log.Printf("%s set admin=%v on user %d", currentUser(r).Username, admin, id)
	}
	s.adminResult(w, r, err, "admin")
}

// deleteUser deletes another user and their documents, once the user
// said Yes to the page's question (static/serverfs.js adds confirm=1). The account goes first: a folder that then can't be
// removed is reported, and CheckNoFolder keeps a new account of the same
// name from inheriting it.
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if r.PostForm.Get("confirm") == "" {
		s.renderUsers(w, r, http.StatusBadRequest, "", "Confirm the deletion: say Yes when asked (the page needs JavaScript).", usersData{})
		return
	}
	target, err := s.cfg.Auth.Store.UserByID(id)
	if err != nil {
		s.adminResult(w, r, err, "deleted")
		return
	}
	if err := s.cfg.Auth.Store.DeleteUser(id); err != nil {
		s.adminResult(w, r, err, "deleted")
		return
	}
	s.cfg.Log.Printf("%s deleted user %q", currentUser(r).Username, target.Username)
	if err := s.cfg.Files.RemoveUser(target.Username); err != nil {
		var ne *files.NameError
		if !errors.As(err, &ne) { // an old-rules name never had a folder
			s.cfg.Log.Printf("remove the files of deleted user %q: %v", target.Username, err)
			s.renderUsers(w, r, http.StatusInternalServerError, "",
				"User deleted, but their files couldn't all be removed: see the server log.", usersData{})
			return
		}
	}
	s.adminResult(w, r, nil, "deleted")
}

// CheckNoFolder refuses a new account named username while a folder of
// that name is left on disk (from an earlier account whose files
// couldn't be removed), so nobody inherits someone else's documents.
func CheckNoFolder(fs *files.Store, username string) error {
	has, err := fs.HasFolder(username)
	var ne *files.NameError
	switch {
	case errors.As(err, &ne):
		return nil // CreateUser reports the bad name
	case err != nil:
		return err
	case has:
		dir, _ := fs.Dir(username)
		return fmt.Errorf("a folder of files for %q is left from an earlier account (%s): remove it first", username, dir)
	}
	return nil
}
