// limits.go holds every limit and tunable of the web server in one
// place: sessions, login throttling, credentials, storage quota, request
// caps and HTTP timeouts. Each name says which area owns it (Session…,
// Login…, Password…, Username…, FileName…, File…, HTTP…). The rules for
// usernames and file names are in web/names.

package web

import "time"

// ---- sessions ----

const (
	// SessionIdle is how long a session lasts without a request; each
	// request moves its end this far ahead (up to SessionMaxAge).
	SessionIdle = 60 * time.Minute

	// SessionMaxAge is the most a session lasts however busy it is: after
	// it, the user logs in again.
	SessionMaxAge = 12 * time.Hour

	// SessionTokenBytes is the number of random bytes in a session token
	// (256 bits). The database keeps only the token's SHA-256.
	SessionTokenBytes = 32

	// SessionCleanupEvery is how often expired sessions and old login
	// attempts are deleted.
	SessionCleanupEvery = 10 * time.Minute

	// SessionIdleWarning is how long before an idle logout the pages warn
	// (static/idle.js): after SessionIdle without a key press, click or
	// touch in any of the user's tabs, the page logs out.
	SessionIdleWarning = 2 * time.Minute

	// SessionKeepAliveEvery is how often a page that is being used (but
	// isn't otherwise talking to the server, as while typing in the app)
	// tells the server so, keeping the session from ending under it.
	SessionKeepAliveEvery = 5 * time.Minute
)

// ---- login throttling (per username) ----

const (
	// LoginSoftAttempts failed logins within LoginSoftWindow make the
	// next one wait until LoginSoftWindow has passed since the last.
	LoginSoftAttempts = 3

	// LoginSoftWindow is the short throttling window.
	LoginSoftWindow = 30 * time.Second

	// LoginHardAttempts failed logins within LoginHardWindow make the
	// next one wait until LoginHardWindow has passed since the last.
	LoginHardAttempts = 6

	// LoginHardWindow is the long throttling window.
	LoginHardWindow = 5 * time.Minute

	// LoginAttemptRetention is how long login attempts are kept (for
	// throttling and for looking into break-in attempts).
	LoginAttemptRetention = 24 * time.Hour
)

// ---- credentials ----

const (
	// PasswordMinLength is the fewest characters a password may have.
	PasswordMinLength = 12

	// PasswordMaxBytes is the most bytes a password may have: bcrypt
	// ignores everything past 72, so longer ones are refused rather than
	// silently cut.
	PasswordMaxBytes = 72

	// PasswordHashCost is the bcrypt cost of stored password hashes
	// (2¹² rounds, about a quarter second on a small server).
	PasswordHashCost = 12

	// UsernameMaxLength is the most characters a username may have
	// (usernames are ASCII, so also the most bytes).
	UsernameMaxLength = 64
)

// ---- storage ----

const (
	// DefaultUserQuotaBytes is the server storage each new user gets for
	// documents: 1 GiB, counting every file in the user's folder.
	DefaultUserQuotaBytes = 1 << 30

	// FileMaxBytes is the largest document that may be stored, whatever
	// the quota left: 256 MiB.
	FileMaxBytes = 256 << 20

	// FileNameMaxLength is the most characters a stored document's name
	// may have, extension included (names are ASCII, so also the most
	// bytes). The other file-name rules are in web/names.
	FileNameMaxLength = 64

	// FileUploadTimeout is how long a client has to send one document
	// (an upload, or a save from the app). It replaces HTTPReadTimeout
	// for those requests: a big file on a slow connection takes minutes.
	FileUploadTimeout = 10 * time.Minute
)

// ---- database ----

// DBBusyTimeout is how long a database statement waits for a lock held by
// another connection before failing.
const DBBusyTimeout = 10 * time.Second

// ---- HTTP ----

const (
	// HTTPFormMaxBytes caps the body of a form post (login, user admin).
	HTTPFormMaxBytes = 64 << 10

	// HTTPReadHeaderTimeout is how long a client has to send its request
	// headers.
	HTTPReadHeaderTimeout = 10 * time.Second

	// HTTPReadTimeout is how long a client has to send a whole request
	// (but see FileUploadTimeout).
	HTTPReadTimeout = 30 * time.Second

	// HTTPWriteTimeout is how long a response may take to go out. It is
	// long because the WebAssembly client is tens of megabytes and may go
	// to a slow connection.
	HTTPWriteTimeout = 10 * time.Minute

	// HTTPIdleTimeout is how long an idle keep-alive connection stays open.
	HTTPIdleTimeout = 2 * time.Minute

	// HTTPShutdownGrace is how long a stopping server waits for requests
	// in flight to finish.
	HTTPShutdownGrace = 15 * time.Second
)
