#!/bin/sh
# KindCommand hook: status + list UI intents.
cat >/dev/null
echo '{"status":"3 findings","list":{"title":"Findings","items":[{"label":"auth.go:12","detail":"nil check","submit":"fix auth.go:12"}]}}'
exit 0
