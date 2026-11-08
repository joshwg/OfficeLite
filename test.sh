#!/usr/bin/env bash
# Run all of OfficeLite's tests.
#
#   ./test.sh [go test flags]     e.g. ./test.sh -v   or   ./test.sh -run TestSubtotal
#   ./test.sh --all [flags]       include the GUI packages (needs the Gio system libraries)
#
# Packages that link the Gio window system (gioui.org/app) need wayland and
# xkbcommon headers to build; without them they are skipped and listed.
# Sample fixtures are grouped under Samples/excel-*/ and Samples/word*/.
# Individual package tests load the fixtures they need from those directories.
set -u
cd "$(dirname "$0")"

all=0
if [ "${1:-}" = "--all" ]; then
	all=1
	shift
fi

if [ "$all" = 1 ]; then
	mapfile -t pkgs < <(go list ./...)
else
	gui=()
	pkgs=()
	while read -r pkg deps; do
		if [[ " $deps " == *" gioui.org/app "* ]]; then
			gui+=("$pkg")
		else
			pkgs+=("$pkg")
		fi
	done < <(go list -f '{{.ImportPath}} {{range .Deps}}{{.}} {{end}}' ./...)
fi

render_pkg=officelite/excel/render
other_pkgs=()
has_render=0
for pkg in "${pkgs[@]}"; do
	if [ "$pkg" = "$render_pkg" ]; then
		has_render=1
	else
		other_pkgs+=("$pkg")
	fi
done

status=0
if [ "${#other_pkgs[@]}" -gt 0 ]; then
	go test "$@" "${other_pkgs[@]}" || status=$?
fi

# This package can take a while when running larger rendering/PDF workloads.
# Verbose mode streams each test's start and completion without making every
# package noisy.
if [ "$has_render" = 1 ]; then
	echo
	echo "Running Excel render tests with per-test progress:"
	go test -v "$@" "$render_pkg" || status=$?
fi

if [ "${all:-0}" = 0 ] && [ "${#gui[@]}" -gt 0 ]; then
	echo
	echo "skipped (need the Gio system libraries; ./test.sh --all to include them):"
	printf '  %s\n' "${gui[@]}"
fi
exit $status
