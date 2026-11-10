#!/bin/sh
# Writes a backup of Nando's databases (core, calories and japanese) at
# $MONGO_URI to the path given: a tar of mongodump's folder, one gzipped BSON
# file per collection. Restore: see README's Backups section.
set -eu
out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT
# Core and every Module (internal/server/databases.go, which gets japanese in
# #37): add a new Module here.
for db in core calories japanese; do
	mongodump --uri "$MONGO_URI" --db "$db" --gzip --out "$out"
done
tar -cf "$1" -C "$out" .
