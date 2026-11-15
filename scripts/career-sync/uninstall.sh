#!/bin/bash
# Unloads and removes the Career Sync's LaunchAgent. Leaves the log, the
# Connector's tokens and the reader's LinkedIn session alone.
set -euo pipefail

label=app.nando.career-sync
launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
rm -f "$HOME/Library/LaunchAgents/$label.plist"
echo "Uninstalled $label."
