// Package web holds the limits of OfficeLite's web server (limits.go):
// session lifetimes, login rate limits, password and username rules,
// request caps and the HTTP server's timeouts, in one place. The server
// itself lives in the packages below it: store (the SQLite database),
// auth (passwords, sessions, login throttling) and server (the HTTP
// handlers); cmd/officeweb runs it. The browser client is the desktop
// app built for WebAssembly (GOOS=js GOARCH=wasm ./cmd/officelite). This
// package imports no other web/ package, so every one of them can use it.
package web
