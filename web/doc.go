// Package web holds the limits of OfficeLite's web server (limits.go):
// session lifetimes, login rate limits, password, username and file-name
// lengths, storage quotas, request caps and the HTTP server's timeouts,
// in one place. The server itself lives in the packages below it: names
// (the username and file-name rules), store (the SQLite database), auth
// (passwords, sessions, login throttling), files (users' documents on
// disk) and server (the HTTP handlers); cmd/officeweb runs it. The browser client is the desktop
// app built for WebAssembly (GOOS=js GOARCH=wasm ./cmd/officelite). This
// package imports no other web/ package, so every one of them can use it.
package web
