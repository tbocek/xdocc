#!/bin/sh
for f in /run/secrets/*; do
	[ -f "$f" ] && export "$(basename "$f")=$(cat "$f")"
done
exec "$@"
