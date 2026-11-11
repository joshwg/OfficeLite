// Sessions and login attempts. A session is found by the SHA-256 of its
// token, so a copy of the database holds no usable session.

package store

import (
	"database/sql"
	"errors"
	"time"
)

// Session is a logged-in browser.
type Session struct {
	User      User
	CreatedAt time.Time
	ExpiresAt time.Time
}

// CreateSession records a session for user userID.
func (s *Store) CreateSession(tokenHash string, userID int64, created, expires time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		tokenHash, userID, ms(created), ms(expires))
	return err
}

// SessionByHash finds the session with this token hash that hasn't
// expired at now, with its user.
func (s *Store) SessionByHash(tokenHash string, now time.Time) (Session, error) {
	row := s.db.QueryRow(`
		SELECT u.id, u.username, u.is_admin, u.quota_bytes, u.created_at, s.created_at, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, ms(now))
	var sess Session
	var userCreated, created, expires int64
	u := &sess.User
	if err := row.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.QuotaBytes, &userCreated, &created, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrNotFound
		}
		return Session{}, err
	}
	u.CreatedAt = fromMS(userCreated)
	sess.CreatedAt = fromMS(created)
	sess.ExpiresAt = fromMS(expires)
	return sess, nil
}

// ExtendSession moves a session's end to expires.
func (s *Store) ExtendSession(tokenHash string, expires time.Time) error {
	_, err := s.db.Exec("UPDATE sessions SET expires_at = ? WHERE token_hash = ?", ms(expires), tokenHash)
	return err
}

// DeleteSession ends one session.
func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

// DeleteExpiredSessions deletes the sessions that ended by now.
func (s *Store) DeleteExpiredSessions(now time.Time) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE expires_at <= ?", ms(now))
	return err
}

// RecordLoginAttempt records a login attempt for throttling and auditing.
func (s *Store) RecordLoginAttempt(username, ip string, ok bool, at time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO login_attempts (username, ip, ok, attempted_at) VALUES (?, ?, ?, ?)",
		username, ip, ok, ms(at))
	return err
}

// FailedLogins returns how many failed logins username (ignoring case)
// had after since, and when the last one was (zero when none).
func (s *Store) FailedLogins(username string, since time.Time) (int, time.Time, error) {
	var n int
	var last sql.NullInt64
	err := s.db.QueryRow(`
		SELECT COUNT(*), MAX(attempted_at) FROM login_attempts
		WHERE username = ? AND ok = 0 AND attempted_at > ?`, username, ms(since)).Scan(&n, &last)
	if err != nil {
		return 0, time.Time{}, err
	}
	if !last.Valid {
		return 0, time.Time{}, nil
	}
	return n, fromMS(last.Int64), nil
}

// DeleteLoginAttemptsBefore deletes the login attempts made before t.
func (s *Store) DeleteLoginAttemptsBefore(t time.Time) error {
	_, err := s.db.Exec("DELETE FROM login_attempts WHERE attempted_at < ?", ms(t))
	return err
}
