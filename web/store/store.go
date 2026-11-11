// Package store is the web server's SQLite database: users, sessions and
// login attempts. Documents aren't in it: they are plain files on disk
// (web/files), described by the file system. It uses the pure-Go modernc.org/sqlite driver, so the server
// builds without CGO. Times are stored as Unix milliseconds.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"officelite/web"
)

// Errors the store returns for expected conditions.
var (
	// ErrNotFound is returned when the user or session asked for doesn't
	// exist.
	ErrNotFound = errors.New("not found")
	// ErrUsernameTaken is returned when a new user's name is in use
	// (names compare without regard to case).
	ErrUsernameTaken = errors.New("username already taken")
	// ErrLastAdmin is returned when a change would leave no administrator.
	ErrLastAdmin = errors.New("can't remove the last administrator")
)

// Store is an open database.
type Store struct {
	db *sql.DB
}

// migrations are the schema changes in order; the database's
// user_version is how many of them it has had.
var migrations = []string{
	// 1: users, sessions, login attempts.
	`CREATE TABLE users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT    NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT    NOT NULL,
		is_admin      INTEGER NOT NULL DEFAULT 0,
		quota_bytes   INTEGER NOT NULL,
		created_at    INTEGER NOT NULL
	);
	CREATE TABLE sessions (
		token_hash TEXT    PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL
	);
	CREATE INDEX sessions_expires ON sessions(expires_at);
	CREATE INDEX sessions_user ON sessions(user_id);
	CREATE TABLE login_attempts (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		username     TEXT    NOT NULL COLLATE NOCASE,
		ip           TEXT    NOT NULL,
		ok           INTEGER NOT NULL,
		attempted_at INTEGER NOT NULL
	);
	CREATE INDEX login_attempts_user ON login_attempts(username, attempted_at);`,
}

// Open opens (creating if missing) the database at path and brings its
// schema up to date.
func Open(path string) (*Store, error) {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", web.DBBusyTimeout.Milliseconds()))
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	// One connection: SQLite has one writer anyway, and this keeps
	// transactions from waiting on each other's locks.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrate applies the migrations the database hasn't had yet, each in
// its own transaction together with the user_version that records it.
func (s *Store) migrate() error {
	var have int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&have); err != nil {
		return err
	}
	if have > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this server's %d", have, len(migrations))
	}
	for i := have; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			return rollback(tx, fmt.Errorf("migration %d: %w", i+1, err))
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			return rollback(tx, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// rollback rolls tx back and returns err, joined with any rollback error.
func rollback(tx *sql.Tx, err error) error {
	if rbErr := tx.Rollback(); rbErr != nil {
		return errors.Join(err, rbErr)
	}
	return err
}

// ms is t as stored: Unix milliseconds.
func ms(t time.Time) int64 { return t.UnixMilli() }

// fromMS is a stored time back as a time.Time.
func fromMS(v int64) time.Time { return time.UnixMilli(v) }
