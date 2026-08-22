#!/bin/sh
# session_before_switch: deny.
cat >/dev/null
echo '{"action":"deny","reason":"dirty repo"}'
exit 0
