// Command officeweb is OfficeLite's web server: it logs users in and
// serves the WebAssembly client (WordLite and ExcelLite in the browser).
// Only administrators add users; the first administrator is made here.
//
//	officeweb -data DIR [-addr HOST:PORT] [-client DIR]   run the server
//	officeweb -data DIR -create-admin NAME                add an administrator
//	officeweb -data DIR -reset-password NAME              set a user's password
//	officeweb -version
//
// DIR holds the database (officeweb.db) and, in a later release, users'
// documents. Each flag can also come from the environment:
// OFFICEWEB_DATA, OFFICEWEB_ADDR, OFFICEWEB_CLIENT. In production it
// listens on loopback behind Apache, which terminates HTTPS.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"officelite/common/version"
	"officelite/web"
	"officelite/web/auth"
	"officelite/web/server"
	"officelite/web/store"
)

// defaultAddr is where the server listens unless told otherwise: loopback
// only, for the reverse proxy in front of it.
const defaultAddr = "127.0.0.1:7100"

// dbFileName is the database's name in the data directory.
const dbFileName = "officeweb.db"

// main parses the flags and runs the server or the account command asked
// for.
func main() {
	dataDir := flag.String("data", os.Getenv("OFFICEWEB_DATA"), "data directory (database, later documents)")
	addr := flag.String("addr", envOr("OFFICEWEB_ADDR", defaultAddr), "address to listen on")
	clientDir := flag.String("client", envOr("OFFICEWEB_CLIENT", "client"), "directory holding officelite.wasm and wasm_exec.js")
	insecure := flag.Bool("insecure-cookies", false, "drop the Secure cookie flag (plain-HTTP testing on a non-localhost host only)")
	createAdmin := flag.String("create-admin", "", "create an administrator `NAME` (asks for the password) and exit")
	resetPassword := flag.String("reset-password", "", "set the password of user `NAME` (asks for it) and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String(), "web server")
		return
	}
	if *dataDir == "" {
		log.Fatal("officeweb: -data (or OFFICEWEB_DATA) is required")
	}
	dir, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, dbFileName))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Print(err)
		}
	}()
	svc := &auth.Service{Store: st}

	switch {
	case *createAdmin != "":
		err = runCreateAdmin(svc, *createAdmin)
	case *resetPassword != "":
		err = runResetPassword(svc, *resetPassword)
	default:
		err = serve(svc, *addr, *clientDir, *insecure)
	}
	if err != nil {
		log.Print(err)
		if closeErr := st.Close(); closeErr != nil {
			log.Print(closeErr)
		}
		os.Exit(1)
	}
}

// envOr returns the environment variable key, or def when it is unset.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// runCreateAdmin adds an administrator, asking for the password.
func runCreateAdmin(svc *auth.Service, name string) error {
	pw, err := readNewPassword()
	if err != nil {
		return err
	}
	u, err := svc.CreateUser(name, pw, true)
	if err != nil {
		return err
	}
	fmt.Printf("Created administrator %q.\n", u.Username)
	return nil
}

// runResetPassword sets a user's password, asking for it; the user's
// sessions end.
func runResetPassword(svc *auth.Service, name string) error {
	u, _, err := svc.Store.UserByName(name)
	if err != nil {
		return fmt.Errorf("user %q: %w", name, err)
	}
	pw, err := readNewPassword()
	if err != nil {
		return err
	}
	if err := svc.SetPassword(u.ID, pw); err != nil {
		return err
	}
	fmt.Printf("Password of %q changed.\n", u.Username)
	return nil
}

// readNewPassword asks for a password twice, without echo when stdin is a
// terminal (otherwise it reads two lines, for scripts).
func readNewPassword() (string, error) {
	read := readLine()
	pw, err := read("Password: ")
	if err != nil {
		return "", err
	}
	if err := auth.ValidatePassword(pw); err != nil {
		return "", err
	}
	again, err := read("Repeat password: ")
	if err != nil {
		return "", err
	}
	if pw != again {
		return "", errors.New("passwords don't match")
	}
	return pw, nil
}

// readLine returns a function that prompts and reads one line from stdin:
// hidden on a terminal, plain otherwise.
func readLine() func(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		return func(prompt string) (string, error) {
			_, _ = fmt.Fprint(os.Stderr, prompt) // a lost prompt is harmless
			b, err := term.ReadPassword(fd)
			_, _ = fmt.Fprintln(os.Stderr)
			return string(b), err
		}
	}
	in := bufio.NewReader(os.Stdin)
	return func(prompt string) (string, error) {
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
}

// serve runs the web server until SIGINT or SIGTERM, cleaning up expired
// sessions in the background.
func serve(svc *auth.Service, addr, clientDir string, insecure bool) error {
	if n, err := svc.Store.UserCount(); err != nil {
		return err
	} else if n == 0 {
		log.Print("no users yet: create the first administrator with officeweb -data DIR -create-admin NAME")
	}
	clientDir, err := filepath.Abs(clientDir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(clientDir, "officelite.wasm")); err != nil {
		log.Printf("warning: no client in %s (build it with ./web.sh build); the app pages won't load", clientDir)
	}
	h, err := server.New(server.Config{Auth: svc, ClientDir: clientDir, InsecureCookies: insecure})
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: web.HTTPReadHeaderTimeout,
		ReadTimeout:       web.HTTPReadTimeout,
		WriteTimeout:      web.HTTPWriteTimeout,
		IdleTimeout:       web.HTTPIdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go cleanup(ctx, svc)

	errc := make(chan error, 1)
	go func() {
		log.Printf("%s web server listening on %s", version.String(), addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), web.HTTPShutdownGrace)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// cleanup deletes expired sessions and old login attempts every
// SessionCleanupEvery until ctx ends.
func cleanup(ctx context.Context, svc *auth.Service) {
	t := time.NewTicker(web.SessionCleanupEvery)
	defer t.Stop()
	for {
		if err := svc.Cleanup(); err != nil {
			log.Printf("cleanup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
