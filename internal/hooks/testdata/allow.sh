#!/bin/sh
# Reads one JSON line from stdin; always allows.
cat >/dev/null
echo '{"action":"allow"}'
exit 0
