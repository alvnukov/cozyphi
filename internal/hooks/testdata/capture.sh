#!/bin/sh
# Reads one JSON line from stdin; writes it to $CAPTURE_OUT.
read -r line
printf '%s' "$line" > "$CAPTURE_OUT"
exit 0
