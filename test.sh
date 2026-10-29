#!/usr/bin/env bash
# Run all of OfficeLite's tests.
#
#   ./test.sh [go test flags]     e.g. ./test.sh -v   or   ./test.sh -run TestSubtotal
#   ./test.sh --all [flags]       include the GUI packages (needs the Gio system libraries)
#
# Packages that link the Gio window system (gioui.org/app) need wayland and
# xkbcommon headers to build; without them they are skipped and listed.
set -u
cd "$(dirname "$0")"

all=0
if [ "${1:-}" = "--all" ]; then
	all=1
	shift
fi

if [ "$all" = 1 ]; then
	exec go test ./... "$@"
fi

gui=()
pkgs=()
while read -r pkg deps; do
	if [[ " $deps " == *" gioui.org/app "* ]]; then
		gui+=("$pkg")
	else
		pkgs+=("$pkg")
	fi
done < <(go list -f '{{.ImportPath}} {{range .Deps}}{{.}} {{end}}' ./...)

go test "${pkgs[@]}" "$@"
status=$?

if [ "${#gui[@]}" -gt 0 ]; then
	echo
	echo "skipped (need the Gio system libraries; ./test.sh --all to include them):"
	printf '  %s\n' "${gui[@]}"
fi
exit $status
