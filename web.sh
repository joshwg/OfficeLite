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

# How often (seconds) a long step reports that it is still running.
PROGRESS_EVERY=10

# step prints what the script is about to do.
step() {
    printf '==> %s\n' "$*"
}

# with_progress runs a command that may take a while: every
# PROGRESS_EVERY seconds it prints how long the command has been running,
# then how long it took. The command runs in the foreground, so Ctrl+C
# stops it; its output and a failure (which stops the script) pass through.
with_progress() {
    local start=$SECONDS ticker status=0
    (
        n=0
        while sleep "$PROGRESS_EVERY"; do
            n=$((n + PROGRESS_EVERY))
            printf '    ... still working (%ds)\n' "$n"
        done
    ) &
    ticker=$!
    # Stop the ticker however the script ends (failure, Ctrl+C).
    trap 'kill "$ticker" 2>/dev/null || true' EXIT
    "$@" || status=$?
    kill "$ticker" 2>/dev/null || true
    wait "$ticker" 2>/dev/null || true
    trap - EXIT
    if ((status != 0)); then
        printf '    failed after %ds\n' $((SECONDS - start)) >&2
        exit "$status"
    fi
    printf '    done in %ds\n' $((SECONDS - start))
}

# build_client builds the WebAssembly client into $OUT/client.
build_client() {
    mkdir -p "$OUT/client"
    step "Building the WebAssembly client (the slowest step, often a minute or more)"
    with_progress env GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o "$OUT/client/officelite.wasm" ./cmd/officelite
    # install, not cp: the toolchain's copy is read-only.
    install -m 0644 "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$OUT/client/"
    rm -f "$OUT/client/officelite.wasm.br" "$OUT/client/officelite.wasm.gz"
    if command -v brotli >/dev/null; then
        step "Compressing the client with brotli (slow at its best setting)"
        with_progress brotli -q 11 -k "$OUT/client/officelite.wasm"
    else
        step "Skipping brotli (not installed: sudo apt install brotli)"
    fi
    step "Compressing the client with gzip"
    with_progress gzip -9 -k "$OUT/client/officelite.wasm"
    ls -l "$OUT/client"
}

# build_server builds the server for GOOS/GOARCH (default: this machine).
build_server() {
    step "Building the server (${GOOS:-$(go env GOOS)}/${GOARCH:-$(go env GOARCH)})"
    with_progress env CGO_ENABLED=0 go build -trimpath -o "$OUT/officeweb" ./cmd/officeweb
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
    step "Serving on http://localhost:7100 (Ctrl+C stops it)"
    # Over http://localhost browsers accept Secure cookies, so no
    # -insecure-cookies is needed.
    exec "$OUT/officeweb" -data "$DATA" -client "$OUT/client" -addr 127.0.0.1:7100
    ;;
admin)
    [[ -n "${2:-}" ]] || { echo "usage: $0 admin NAME" >&2; exit 2; }
    build_server
    mkdir -p "$DATA"
    step "Creating administrator $2"
    exec "$OUT/officeweb" -data "$DATA" -create-admin "$2"
    ;;
package)
    build_client
    GOOS=linux GOARCH=amd64 build_server
    version=$(sed -n 's/^const Version = "\(.*\)"/\1/p' common/version/version.go)
    pkg="$ROOT/bin/officeweb-$version"
    step "Packaging version $version"
    rm -rf "$pkg"
    mkdir -p "$pkg"
    cp -r "$OUT/officeweb" "$OUT/client" web/deploy/* "$pkg/"
    with_progress tar -C "$ROOT/bin" -czf "$pkg.tar.gz" "officeweb-$version"
    echo "packaged $pkg.tar.gz"
    ;;
*)
    sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac
