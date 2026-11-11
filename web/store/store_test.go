// Tests of the store: schema setup, users, the last-administrator guard,
// sessions and login attempts.

package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// openTest opens a fresh database in a temporary directory.
func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

// TestReopen checks that opening an existing database again keeps its
// data and doesn't rerun migrations.
func TestReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("ann", "h", true, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if n, err := s.UserCount(); err != nil || n != 1 {
		t.Fatalf("UserCount = %d, %v; want 1", n, err)
	}
}

// TestUsers checks creating, finding (case-insensitively) and the
// duplicate-name error.
func TestUsers(t *testing.T) {
	s := openTest(t)
	now := time.UnixMilli(1_700_000_000_000)
	u, err := s.CreateUser("Ann", "hash1", false, 1<<30, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("ANN", "hash2", false, 1, now); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate name: err = %v, want ErrUsernameTaken", err)
	}
	got, hash, err := s.UserByName("ann")
	if err != nil {
		t.Fatal(err)
	}
	if got != u || hash != "hash1" {
		t.Fatalf("UserByName = %+v %q, want %+v hash1", got, hash, u)
	}
	if _, _, err := s.UserByName("bob"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: err = %v, want ErrNotFound", err)
	}
}

// TestLastAdmin checks that the last administrator can't be demoted or
// deleted, and another one can.
func TestLastAdmin(t *testing.T) {
	s := openTest(t)
	now := time.Now()
	a, _ := s.CreateUser("a", "h", true, 1, now)
	if err := s.SetAdmin(a.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: err = %v", err)
	}
	if err := s.DeleteUser(a.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin: err = %v", err)
	}
	b, _ := s.CreateUser("b", "h", false, 1, now)
	if err := s.SetAdmin(b.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(a.ID); err != nil {
		t.Fatalf("delete with another admin: %v", err)
	}
	if err := s.DeleteUser(a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: err = %v", err)
	}
}

// TestSessions checks session lookup, expiry, and that a password change
// or a deleted user ends sessions.
func TestSessions(t *testing.T) {
	s := openTest(t)
	now := time.UnixMilli(1_700_000_000_000)
	u, _ := s.CreateUser("ann", "h", false, 1, now)
	if err := s.CreateSession("tok", u.ID, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	sess, err := s.SessionByHash("tok", now.Add(time.Minute))
	if err != nil || sess.User.ID != u.ID {
		t.Fatalf("SessionByHash = %+v, %v", sess, err)
	}
	if _, err := s.SessionByHash("tok", now.Add(time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: err = %v", err)
	}
	if err := s.SetPasswordHash(u.ID, "h2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash("tok", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session after password change: err = %v", err)
	}
	if err := s.CreateSession("tok2", u.ID, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash("tok2", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session of deleted user: err = %v", err)
	}
}

// TestFailedLogins checks counting failures in a window.
func TestFailedLogins(t *testing.T) {
	s := openTest(t)
	now := time.UnixMilli(1_700_000_000_000)
	for i := range 3 {
		if err := s.RecordLoginAttempt("Ann", "1.2.3.4", false, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordLoginAttempt("ann", "1.2.3.4", true, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	n, last, err := s.FailedLogins("ANN", now.Add(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || !last.Equal(now.Add(2*time.Second)) {
		t.Fatalf("FailedLogins = %d, %v; want 2, %v", n, last, now.Add(2*time.Second))
	}
	if n, _, _ := s.FailedLogins("bob", now.Add(-time.Hour)); n != 0 {
		t.Fatalf("FailedLogins(bob) = %d", n)
	}
}
