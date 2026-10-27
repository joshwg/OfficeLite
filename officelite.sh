#!/usr/bin/env bash
# Build and run OfficeLite.
#
#   ./officelite.sh [--excel|--word] [file ...]    build (if needed) and open
#   ./officelite.sh --linux [...]                  native Linux build instead
#   ./officelite.sh --build                        build only
#
# Under WSL the default is the native Windows .exe (no extra packages needed;
# it shows up as a normal Windows window). The Linux build needs the GUI
# development headers:
#   sudo apt install gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev \
#     libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libffi-dev \
#     libxcursor-dev libvulkan-dev
set -euo pipefail

cd "$(dirname "$0")"
ROOT=$(pwd)
mkdir -p bin

target=windows
build_only=false
if ! grep -qi microsoft /proc/version 2>/dev/null; then
    target=linux # not WSL: build for this machine
fi
case "${1:-}" in
--linux) target=linux; shift ;;
--windows) target=windows; shift ;;
--build) build_only=true; shift ;;
esac

if [[ $target == windows ]]; then
    exe="$ROOT/bin/OfficeLite.exe"
    GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui" -o "$exe" ./cmd/officelite
    $build_only && { echo "built $exe"; exit 0; }
    args=()
    for a in "$@"; do
        if [[ $a == --* ]]; then
            args+=("$a")
        else
            args+=("$(wslpath -w "$(realpath "$a")")")
        fi
    done
    # Detach so the terminal is free while the windows are open.
    (cd /mnt/c && setsid "$exe" "${args[@]}" >/dev/null 2>&1 &)
else
    if ! pkg-config --exists xkbcommon wayland-client egl 2>/dev/null; then
        echo "Missing GUI headers for the Linux build. Install them with:" >&2
        sed -n '/sudo apt install/,/libvulkan-dev/p' "$0" | sed 's/^#//' >&2
        exit 1
    fi
    exe="$ROOT/bin/officelite"
    go build -o "$exe" ./cmd/officelite
    $build_only && { echo "built $exe"; exit 0; }
    # Under WSL, borrow the Windows fonts (Calibri, Segoe UI, ...).
    if [[ -d /mnt/c/Windows/Fonts ]]; then
        export OFFICELITE_FONTS="${OFFICELITE_FONTS:+$OFFICELITE_FONTS:}/mnt/c/Windows/Fonts"
    fi
    exec "$exe" "$@"
fi
