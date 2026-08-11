#!/bin/sh
# Echo whether PHI_API_KEY leaked into the environment.
if [ -n "$PHI_API_KEY" ]; then
  echo '{"action":"deny","reason":"api key leaked"}'
  exit 0
fi
if [ "$PHI_HOOK_EVENT" != "pre_tool" ]; then
  echo '{"action":"deny","reason":"missing hook event"}'
  exit 0
fi
echo '{"action":"allow","context":"env ok"}'
exit 0
