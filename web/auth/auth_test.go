// Tests of logins, throttling, session lifetimes and password changes,
// with a fake clock.

package auth

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"officelite/web"
	"officelite/web/store"
)

// testPassword passes ValidatePassword.
const testPassword = "correct horse battery"

// clock is a settable fake time.
type clock struct{ t time.Time }

// now returns the fake time.
func (c *clock) now() time.Time { return c.t }

// newTest returns a service on a fresh database with user "ann", and its
// clock.
func newTest(t *testing.T) (*Service, *clock) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	c := &clock{t: time.UnixMilli(1_700_000_000_000)}
	s := &Service{Store: st, Now: c.now}
	if _, err := s.CreateUser("ann", testPassword, false); err != nil {
		t.Fatal(err)
	}
	return s, c
}

// TestValidate checks the username and password rules.
func TestValidate(t *testing.T) {
	for _, name := range []string{"", "a b", "x/y", "../etc", "josé", "a@b", ".x"} {
		if ValidateUsername(name) == nil {
			t.Errorf("ValidateUsername(%q) accepted", name)
		}
	}
	for _, name := range []string{"ann", "j.smith", "Josh", "a_b-c"} {
		if err := ValidateUsername(name); err != nil {
			t.Errorf("ValidateUsername(%q): %v", name, err)
		}
	}
	if ValidatePassword("short") == nil {
		t.Error("short password accepted")
	}
	long := make([]byte, web.PasswordMaxBytes+1)
	for i := range long {
		long[i] = 'x'
	}
	if ValidatePassword(string(long)) == nil {
		t.Error("password over 72 bytes accepted")
	}
}

// TestLogin checks a good login, a wrong password and an unknown user.
func TestLogin(t *testing.T) {
	s, _ := newTest(t)
	token, u, err := s.Login("ANN", testPassword, "10.0.0.1")
	if err != nil || u.Username != "ann" || token == "" {
		t.Fatalf("Login = %q, %+v, %v", token, u, err)
	}
	sess, err := s.Authenticate(token)
	if err != nil || sess.User.ID != u.ID {
		t.Fatalf("Authenticate = %+v, %v", sess, err)
	}
	if _, _, err := s.Login("ann", "wrong password!", "10.0.0.1"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("wrong password: err = %v", err)
	}
	if _, _, err := s.Login("nobody", testPassword, "10.0.0.1"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("unknown user: err = %v", err)
	}
	if _, err := s.Authenticate("not-a-token"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("bad token: err = %v", err)
	}
	if err := s.Logout(token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("after logout: err = %v", err)
	}
}

// TestThrottle checks the cooldowns: LoginSoftAttempts failures block
// even the right password for LoginSoftWindow; an attempt during the
// cooldown counts as a failure and restarts it; LoginHardAttempts
// failures block for LoginHardWindow.
func TestThrottle(t *testing.T) {
	s, c := newTest(t)
	// try logs in with password and returns the throttle wait (0 when
	// not throttled), failing the test on any other unexpected error.
	try := func(password string) time.Duration {
		t.Helper()
		_, _, err := s.Login("ann", password, "ip")
		var th *ThrottledError
		switch {
		case errors.As(err, &th):
			return th.Wait
		case err == nil, errors.Is(err, ErrBadCredentials):
			return 0
		}
		t.Fatal(err)
		return 0
	}
	for range web.LoginSoftAttempts {
		if w := try("wrong password!"); w != 0 {
			t.Fatalf("throttled too early: %v", w)
		}
		c.t = c.t.Add(time.Second)
	}
	// 3 failures: the right password is refused for the soft window.
	if w := try(testPassword); w != web.LoginSoftWindow {
		t.Fatalf("after %d failures: wait %v, want %v", web.LoginSoftAttempts, w, web.LoginSoftWindow)
	}
	// That attempt counted: 20 s later the wait is from it, not from the
	// last wrong password (so 10 s, and this one counts again).
	c.t = c.t.Add(20 * time.Second)
	if w := try(testPassword); w != web.LoginSoftWindow {
		t.Fatalf("retry during cooldown: wait %v, want it restarted at %v", w, web.LoginSoftWindow)
	}
	// 5 failures so far; after the soft wait one more wrong password
	// makes 6: the hard window.
	c.t = c.t.Add(web.LoginSoftWindow)
	if w := try("wrong password!"); w != 0 {
		t.Fatalf("after the soft wait: still throttled %v", w)
	}
	c.t = c.t.Add(web.LoginSoftWindow)
	if w := try(testPassword); w <= web.LoginSoftWindow {
		t.Fatalf("after %d failures: wait %v, want the hard window", web.LoginHardAttempts, w)
	}
	c.t = c.t.Add(web.LoginHardWindow)
	if w := try(testPassword); w != 0 {
		t.Fatalf("after the hard window: still throttled %v", w)
	}
}

// TestThrottledMessage checks the wording and rounding of the wait.
func TestThrottledMessage(t *testing.T) {
	for wait, want := range map[time.Duration]string{
		29*time.Second + 100*time.Millisecond: "Please wait 30 seconds before",
		4*time.Minute + 59*time.Second:        "Please wait 4 minutes and 59 seconds before",
	} {
		if got := (&ThrottledError{Wait: wait}).Error(); !strings.Contains(got, want) {
			t.Errorf("wait %v: %q, want %q", wait, got, want)
		}
	}
}

// TestSessionLifetime checks the idle timeout, its sliding, and the
// maximum age.
func TestSessionLifetime(t *testing.T) {
	s, c := newTest(t)
	token, _, err := s.Login("ann", testPassword, "ip")
	if err != nil {
		t.Fatal(err)
	}
	// Active every half idle period: survives until SessionMaxAge.
	start := c.t
	for c.t.Sub(start) < web.SessionMaxAge-web.SessionIdle/2 {
		c.t = c.t.Add(web.SessionIdle / 2)
		if _, err := s.Authenticate(token); err != nil {
			t.Fatalf("at %v: %v", c.t.Sub(start), err)
		}
	}
	c.t = start.Add(web.SessionMaxAge)
	if _, err := s.Authenticate(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("past max age: err = %v", err)
	}

	token, _, err = s.Login("ann", testPassword, "ip")
	if err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(web.SessionIdle)
	if _, err := s.Authenticate(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("idle session: err = %v", err)
	}
}

// TestChangePassword checks the current-password check and that a change
// ends existing sessions.
func TestChangePassword(t *testing.T) {
	s, _ := newTest(t)
	token, u, err := s.Login("ann", testPassword, "ip")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassword(u.ID, "wrong password!", "another long password"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := s.ChangePassword(u.ID, testPassword, "another long password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("old session after change: err = %v", err)
	}
	if _, _, err := s.Login("ann", "another long password", "ip"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

// TestUsernameCase checks that a username keeps its case but compares
// without regard to it: logging in with any case works, and no second
// account may differ only in case.
func TestUsernameCase(t *testing.T) {
	s, _ := newTest(t)
	created, err := s.CreateUser("Josh", testPassword, false)
	if err != nil || created.Username != "Josh" {
		t.Fatalf("CreateUser = %+v, %v", created, err)
	}
	if _, err := s.CreateUser("jOSH", testPassword, false); !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("second Josh: err = %v", err)
	}
	_, u, err := s.Login("JOSH", testPassword, "10.0.0.1")
	if err != nil || u.Username != "Josh" {
		t.Fatalf("Login = %+v, %v", u, err)
	}
}
