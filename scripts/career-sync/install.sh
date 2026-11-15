#!/bin/bash
# Installs the Career Sync's LaunchAgent for the signed-in user: fills in the
# paths in app.nando.career-sync.plist and loads it. Safe to run again.
set -euo pipefail

label=app.nando.career-sync
here="$(cd "$(dirname "$0")" && pwd)"
uv="$(command -v uv)" || { echo "uv isn't installed: https://docs.astral.sh/uv/" >&2; exit 1; }
agent="$HOME/Library/LaunchAgents/$label.plist"
log="$HOME/Library/Logs/nando-career-sync.log"

mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
sed -e "s|@UV@|$uv|" \
	-e "s|@UV_DIR@|$(dirname "$uv")|" \
	-e "s|@SCRIPT@|$here/sync.py|" \
	-e "s|@LOG@|$log|" \
	"$here/$label.plist" >"$agent"
plutil -lint "$agent" >/dev/null

# bootout first, so running it again picks up a changed plist.
launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$agent"
echo "Installed $label. It runs now, then every hour. Log: $log"
