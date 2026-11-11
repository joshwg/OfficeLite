// Package auth checks credentials and keeps sessions for the web server:
// bcrypt password hashes, login throttling per username, random session
// tokens (the database keeps only their SHA-256) with an idle timeout and
// a maximum age. Accounts are made only by administrators (or by the
// officeweb command line); there is no sign-up.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"officelite/web"
	"officelite/web/names"
	"officelite/web/store"
)

// Errors returned to the handlers, which show them to the user.
var (
	// ErrBadCredentials is returned for a wrong username or password; it
	// doesn't say which.
	ErrBadCredentials = errors.New("invalid username or password")
	// ErrNoSession is returned for a missing, unknown or expired session.
	ErrNoSession = errors.New("not logged in")
)

// ThrottledError is returned by Login while a username is throttled.
type ThrottledError struct {
	Wait time.Duration // until the next attempt is allowed
}

// Error says how long to wait, in minutes and seconds.
func (e *ThrottledError) Error() string {
	secs := e.Seconds()
	if secs >= 60 {
		return fmt.Sprintf("Too many failed login attempts. Please wait %d minutes and %d seconds before trying again.", secs/60, secs%60)
	}
	return fmt.Sprintf("Too many failed login attempts. Please wait %d seconds before trying again.", secs)
}

// Seconds is the wait in whole seconds, rounded up so that a client
// counting it down doesn't try a moment early.
func (e *ThrottledError) Seconds() int {
	return int((e.Wait + time.Second - 1) / time.Second)
}

// Service checks logins and sessions against a store.
type Service struct {
	Store *store.Store
	// Now is the clock; nil uses time.Now (tests set it).
	Now func() time.Time
}

// now returns the current time from s.Now or the system clock.
func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// dummyHash is compared against when the username doesn't exist, so a
// wrong name takes as long as a wrong password.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("officelite-no-such-user"), web.PasswordHashCost)

// HashPassword returns the bcrypt hash of a password that passed
// ValidatePassword.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), web.PasswordHashCost)
	return string(h), err
}

// ValidatePassword returns why password isn't acceptable, or nil.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < web.PasswordMinLength {
		return fmt.Errorf("password must have at least %d characters", web.PasswordMinLength)
	}
	if len(password) > web.PasswordMaxBytes {
		return fmt.Errorf("password must be at most %d bytes", web.PasswordMaxBytes)
	}
	return nil
}

// ValidateUsername returns why username isn't acceptable, or nil (the
// rules are in web/names).
func ValidateUsername(username string) error { return names.Username(username) }

// newToken returns a new random session token and its hash.
func newToken() (token, hash string, err error) {
	b := make([]byte, web.SessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

// hashToken is the database key of a session token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateUser validates and adds an account with the default quota.
func (s *Service) CreateUser(username, password string, admin bool) (store.User, error) {
	username = strings.TrimSpace(username)
	if err := ValidateUsername(username); err != nil {
		return store.User{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return store.User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.User{}, err
	}
	return s.Store.CreateUser(username, hash, admin, web.DefaultUserQuotaBytes, s.now())
}

// SetPassword validates and sets an account's password, ending all its
// sessions (an administrator's reset, or the command line's).
func (s *Service) SetPassword(userID int64, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.Store.SetPasswordHash(userID, hash)
}

// ChangePassword sets a user's own password after checking the current
// one; all the user's sessions end, so the caller starts a new one.
func (s *Service) ChangePassword(userID int64, current, password string) error {
	hash, err := s.Store.PasswordHash(userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return errors.New("current password is wrong")
	}
	return s.SetPassword(userID, password)
}

// Login checks a username and password from ip and starts a session,
// returning its token. A throttled username gets a *ThrottledError
// without its password being checked.
func (s *Service) Login(username, password, ip string) (string, store.User, error) {
	username = strings.TrimSpace(username)
	now := s.now()
	if wait, err := s.throttle(username, now); err != nil {
		return "", store.User{}, err
	} else if wait > 0 {
		// An attempt while throttled counts as a failure (as on the
		// gemini_pwd site): it restarts the wait and counts toward
		// LoginHardAttempts, so guessing through the cooldown doesn't pay.
		if err := s.Store.RecordLoginAttempt(username, ip, false, now); err != nil {
			return "", store.User{}, err
		}
		wait, err = s.throttle(username, now)
		if err != nil {
			return "", store.User{}, err
		}
		return "", store.User{}, &ThrottledError{Wait: wait}
	}
	user, hash, err := s.Store.UserByName(username)
	switch {
	case errors.Is(err, store.ErrNotFound):
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password)) // same timing as a wrong password
	case err != nil:
		return "", store.User{}, err
	}
	ok := err == nil && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	if err := s.Store.RecordLoginAttempt(username, ip, ok, now); err != nil {
		return "", store.User{}, err
	}
	if !ok {
		return "", store.User{}, ErrBadCredentials
	}
	token, err := s.StartSession(user.ID)
	if err != nil {
		return "", store.User{}, err
	}
	return token, user, nil
}

// throttle returns how long username must wait before its next login
// attempt: zero unless it had LoginSoftAttempts failures within
// LoginSoftWindow or LoginHardAttempts within LoginHardWindow.
func (s *Service) throttle(username string, now time.Time) (time.Duration, error) {
	n, last, err := s.Store.FailedLogins(username, now.Add(-web.LoginHardWindow))
	if err != nil {
		return 0, err
	}
	if n >= web.LoginHardAttempts {
		return last.Add(web.LoginHardWindow).Sub(now), nil
	}
	n, last, err = s.Store.FailedLogins(username, now.Add(-web.LoginSoftWindow))
	if err != nil {
		return 0, err
	}
	if n >= web.LoginSoftAttempts {
		return last.Add(web.LoginSoftWindow).Sub(now), nil
	}
	return 0, nil
}

// StartSession starts a session for a user and returns its token.
func (s *Service) StartSession(userID int64) (string, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	if err := s.Store.CreateSession(hash, userID, now, now.Add(web.SessionIdle)); err != nil {
		return "", err
	}
	return token, nil
}

// Authenticate returns the session for token, its end moved SessionIdle
// ahead (but not past SessionMaxAge from its start).
func (s *Service) Authenticate(token string) (store.Session, error) {
	if token == "" {
		return store.Session{}, ErrNoSession
	}
	hash := hashToken(token)
	now := s.now()
	sess, err := s.Store.SessionByHash(hash, now)
	if errors.Is(err, store.ErrNotFound) {
		return store.Session{}, ErrNoSession
	} else if err != nil {
		return store.Session{}, err
	}
	end := sess.CreatedAt.Add(web.SessionMaxAge)
	if !now.Before(end) {
		if err := s.Store.DeleteSession(hash); err != nil {
			return store.Session{}, err
		}
		return store.Session{}, ErrNoSession
	}
	expires := now.Add(web.SessionIdle)
	if expires.After(end) {
		expires = end
	}
	if err := s.Store.ExtendSession(hash, expires); err != nil {
		return store.Session{}, err
	}
	sess.ExpiresAt = expires
	return sess, nil
}

// Logout ends the session for token.
func (s *Service) Logout(token string) error {
	if token == "" {
		return nil
	}
	return s.Store.DeleteSession(hashToken(token))
}

// Cleanup deletes expired sessions and login attempts older than
// LoginAttemptRetention.
func (s *Service) Cleanup() error {
	now := s.now()
	return errors.Join(
		s.Store.DeleteExpiredSessions(now),
		s.Store.DeleteLoginAttemptsBefore(now.Add(-web.LoginAttemptRetention)),
	)
}
