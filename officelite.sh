#!/usr/bin/env bash
# Build and run OfficeLite (the desktop app; the web version is web.sh).
#
#   ./officelite.sh [--excel|--word] [--edit] [file ...]   build, then open
#   ./officelite.sh --linux [app options] [file ...]       native Linux build instead
#   ./officelite.sh --windows [app options] [file ...]     Windows .exe (the WSL default)
#   ./officelite.sh --apple                                build for Apple Silicon only
#   ./officelite.sh --build                                build only, for this machine
#   ./officelite.sh --help                                 show this help
#
# Only the first option picks the build; the rest go to the app:
#   --excel   open the files as spreadsheets; with none, a blank workbook
#   --word    open the files as documents; with none, a blank document
#   --edit    open Word templates (.dotx, .dot) for editing, not as new documents
#
# On macOS the default is a native build for this Mac's CPU (Apple Silicon or
# Intel), opened directly; it needs Xcode's command-line tools for CGO.
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
if [[ $(uname -s) == Darwin ]]; then
    target=mac
fi
case "${1:-}" in
-h | --help | help)
    # The comment block at the top, without its "# ".
    awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
    exit 0
    ;;
--linux) target=linux; shift ;;
--windows) target=windows; shift ;;
--apple) target=apple; shift ;;
--build) build_only=true; shift ;;
esac

if [[ $target == apple ]]; then
    if [[ $(go env GOOS) != darwin ]]; then
        echo "--apple requires macOS and its Apple SDK to build for Apple Silicon." >&2
        exit 1
    fi
    exe="$ROOT/bin/OfficeLite-Apple-Silicon"
    GOOS=darwin GOARCH=arm64 go build -o "$exe" ./cmd/officelite
    echo "built $exe"
    exit 0
elif [[ $target == mac ]]; then
    case $(uname -m) in
    arm64) exe="$ROOT/bin/OfficeLite-Apple-Silicon" ;;
    *) exe="$ROOT/bin/OfficeLite-Intel" ;;
    esac
    CGO_ENABLED=1 go build -o "$exe" ./cmd/officelite
    $build_only && { echo "built $exe"; exit 0; }
    exec "$exe" "$@"
elif [[ $target == windows ]]; then
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
    if ! pkg-config --exists xkbcommon xkbcommon-x11 wayland-client egl glesv2 x11-xcb xcursor vulkan 2>/dev/null; then
        echo "Missing GUI headers for the Linux build. Install them with:" >&2
        sed -n '/^#.*sudo apt install/,/^#.*libvulkan-dev/p' "$0" | sed 's/^#//' >&2
        exit 1
    fi
    exe="$ROOT/bin/officelite"
    go build -o "$exe" ./cmd/officelite
    echo "For hardware rendering, run with: LIBGL_ALWAYS_SOFTWARE=0 ./officelite.sh --linux"
    $build_only && { echo "built $exe"; exit 0; }
    # Under WSL, borrow the Windows fonts (Calibri, Segoe UI, ...).
    if [[ -d /mnt/c/Windows/Fonts ]]; then
        export OFFICELITE_FONTS="${OFFICELITE_FONTS:+$OFFICELITE_FONTS:}/mnt/c/Windows/Fonts"
    fi
    # Avoid Mesa's DRI3 device warning on Linux displays without DRI3.
    # Preserve an explicit setting for systems that support hardware rendering.
    export LIBGL_ALWAYS_SOFTWARE="${LIBGL_ALWAYS_SOFTWARE:-1}"
    exec "$exe" "$@"
fi
