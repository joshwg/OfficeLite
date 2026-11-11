#!/usr/bin/env bash
# Build and run OfficeLite's web server (cmd/officeweb) and its
# WebAssembly client (cmd/officelite built with GOOS=js GOARCH=wasm).
#
#   ./web.sh build                 client + Linux server into bin/web/
#   ./web.sh run                   build, then serve on http://localhost:7100
#                                  with data in bin/webdata/
#   ./web.sh admin NAME            create an administrator in bin/webdata/
#   ./web.sh package               build for the server (linux/amd64) into
#                                  bin/officeweb-<version>.tar.gz
#
# The client is precompressed (officelite.wasm.br with brotli when it is
# installed — sudo apt install brotli — and .gz always) so the server can
# send the small copy.
set -euo pipefail

cd "$(dirname "$0")"
ROOT=$(pwd)
OUT="$ROOT/bin/web"
DATA="$ROOT/bin/webdata"

# build_client builds the WebAssembly client into $OUT/client.
build_client() {
    mkdir -p "$OUT/client"
    GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o "$OUT/client/officelite.wasm" ./cmd/officelite
    # install, not cp: the toolchain's copy is read-only.
    install -m 0644 "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$OUT/client/"
    rm -f "$OUT/client/officelite.wasm.br" "$OUT/client/officelite.wasm.gz"
    if command -v brotli >/dev/null; then
        brotli -q 11 -k "$OUT/client/officelite.wasm"
    fi
    gzip -9 -k "$OUT/client/officelite.wasm"
    ls -l "$OUT/client"
}

# build_server builds the server for GOOS/GOARCH (default: this machine).
build_server() {
    CGO_ENABLED=0 go build -trimpath -o "$OUT/officeweb" ./cmd/officeweb
}

case "${1:-}" in
build)
    build_client
    build_server
    echo "built $OUT"
    ;;
run)
    build_client
    build_server
    mkdir -p "$DATA"
    # Over http://localhost browsers accept Secure cookies, so no
    # -insecure-cookies is needed.
    exec "$OUT/officeweb" -data "$DATA" -client "$OUT/client" -addr 127.0.0.1:7100
    ;;
admin)
    [[ -n "${2:-}" ]] || { echo "usage: $0 admin NAME" >&2; exit 2; }
    build_server
    mkdir -p "$DATA"
    exec "$OUT/officeweb" -data "$DATA" -create-admin "$2"
    ;;
package)
    build_client
    GOOS=linux GOARCH=amd64 build_server
    version=$(sed -n 's/^const Version = "\(.*\)"/\1/p' common/version/version.go)
    pkg="$ROOT/bin/officeweb-$version"
    rm -rf "$pkg"
    mkdir -p "$pkg"
    cp -r "$OUT/officeweb" "$OUT/client" web/deploy/* "$pkg/"
    tar -C "$ROOT/bin" -czf "$pkg.tar.gz" "officeweb-$version"
    echo "packaged $pkg.tar.gz"
    ;;
*)
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac
