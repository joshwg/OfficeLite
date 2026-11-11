// Users: creating, finding, listing, changing and deleting accounts.
// Changing a password ends the user's sessions in the same transaction,
// and no change may leave the server without an administrator.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// User is an account.
type User struct {
	ID         int64
	Username   string
	IsAdmin    bool
	QuotaBytes int64 // server storage for documents
	CreatedAt  time.Time
}

// userColumns are the columns scanUser reads, in its order.
const userColumns = "id, username, is_admin, quota_bytes, created_at"

// scanner is a *sql.Row or *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

// scanUser reads userColumns from row.
func scanUser(row scanner) (User, error) {
	var u User
	var created int64
	if err := row.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.QuotaBytes, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	u.CreatedAt = fromMS(created)
	return u, nil
}

// CreateUser adds an account with the given password hash.
func (s *Store) CreateUser(username, passwordHash string, admin bool, quotaBytes int64, now time.Time) (User, error) {
	res, err := s.db.Exec(
		"INSERT INTO users (username, password_hash, is_admin, quota_bytes, created_at) VALUES (?, ?, ?, ?, ?)",
		username, passwordHash, admin, quotaBytes, ms(now))
	if err != nil {
		// The driver reports "UNIQUE constraint failed: users.username".
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, ErrUsernameTaken
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, IsAdmin: admin, QuotaBytes: quotaBytes, CreatedAt: fromMS(ms(now))}, nil
}

// UserByName finds an account by name (ignoring case) and returns it with
// its password hash.
func (s *Store) UserByName(username string) (User, string, error) {
	row := s.db.QueryRow("SELECT "+userColumns+", password_hash FROM users WHERE username = ?", username)
	var u User
	var created int64
	var hash string
	if err := row.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.QuotaBytes, &created, &hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, "", ErrNotFound
		}
		return User{}, "", err
	}
	u.CreatedAt = fromMS(created)
	return u, hash, nil
}

// UserByID finds an account by ID.
func (s *Store) UserByID(id int64) (User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

// PasswordHash returns an account's password hash.
func (s *Store) PasswordHash(id int64) (string, error) {
	var hash string
	if err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return hash, nil
}

// Users lists all accounts by name.
func (s *Store) Users() ([]User, error) {
	rows, err := s.db.Query("SELECT " + userColumns + " FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UserCount returns how many accounts there are.
func (s *Store) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// SetPasswordHash replaces an account's password hash and ends all its
// sessions.
func (s *Store) SetPasswordHash(id int64, hash string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	res, err := tx.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
	if err != nil {
		return rollback(tx, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return rollback(tx, err)
	} else if n == 0 {
		return rollback(tx, ErrNotFound)
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return rollback(tx, err)
	}
	return tx.Commit()
}

// SetAdmin makes an account an administrator or not. Taking it from the
// last administrator fails with ErrLastAdmin.
func (s *Store) SetAdmin(id int64, admin bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if !admin {
		if err := checkNotLastAdmin(tx, id); err != nil {
			return rollback(tx, err)
		}
	}
	res, err := tx.Exec("UPDATE users SET is_admin = ? WHERE id = ?", admin, id)
	if err != nil {
		return rollback(tx, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return rollback(tx, err)
	} else if n == 0 {
		return rollback(tx, ErrNotFound)
	}
	return tx.Commit()
}

// DeleteUser deletes an account and its sessions. Deleting the last
// administrator fails with ErrLastAdmin.
func (s *Store) DeleteUser(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := checkNotLastAdmin(tx, id); err != nil {
		return rollback(tx, err)
	}
	res, err := tx.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return rollback(tx, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return rollback(tx, err)
	} else if n == 0 {
		return rollback(tx, ErrNotFound)
	}
	return tx.Commit()
}

// checkNotLastAdmin returns ErrLastAdmin when account id is the only
// administrator.
func checkNotLastAdmin(tx *sql.Tx, id int64) error {
	var isAdmin bool
	if err := tx.QueryRow("SELECT is_admin FROM users WHERE id = ?", id).Scan(&isAdmin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !isAdmin {
		return nil
	}
	var admins int
	if err := tx.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = 1").Scan(&admins); err != nil {
		return err
	}
	if admins <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// SetQuota sets how many bytes of documents an account may keep on the
// server.
func (s *Store) SetQuota(id, quotaBytes int64) error {
	res, err := s.db.Exec("UPDATE users SET quota_bytes = ? WHERE id = ?", quotaBytes, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
