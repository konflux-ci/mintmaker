#!/bin/sh
LOG_FILE="${LOG_FILE:-/workspace/shared-data/renovate-logs.json}"
FAIL_MSG='Sanitization step failed; the entire logs of this execution were removed for caution'
fail_safe() {
  echo "$FAIL_MSG"
  echo "$FAIL_MSG" > "$LOG_FILE"
  exit 0
}
if [ ! -f "$LOG_FILE" ]; then
  echo 'Log file not found, skipping sanitization'
  exit 0
fi
echo 'Scanning log file for leaked secrets...'
SCAN_OUTPUT=$(leaktk scan --kind JSONData "@${LOG_FILE}" 2>&1)
if [ $? -ne 0 ]; then
  echo "leaktk scan failed: $SCAN_OUTPUT"
  fail_safe
fi
if [ -z "$SCAN_OUTPUT" ]; then
  echo 'No secrets detected or scanner produced no output'
  exit 0
fi
SECRETS=$(echo "$SCAN_OUTPUT" | python3 -c "
import sys, json
for r in json.load(sys.stdin).get('results',[]):
    s = r.get('secret','')
    if s: print(s)
")
if [ $? -ne 0 ]; then
  echo 'Failed to parse leaktk output'
  fail_safe
fi
if [ -z "$SECRETS" ]; then
  echo 'No secrets found in log file'
  exit 0
fi
if ! cp "$LOG_FILE" "${LOG_FILE}.tmp"; then
  echo 'Failed to create temporary log file'
  fail_safe
fi
REDACT_FAILED=0
while IFS= read -r secret; do
  if [ -n "$secret" ]; then
    ESCAPED=$(printf '%s\n' "$secret" | sed 's/[[\.*^$|\\]/\\&/g')
    if ! sed -i "s|${ESCAPED}|**REDACTED**|g" "${LOG_FILE}.tmp"; then
      REDACT_FAILED=1
      break
    fi
  fi
done <<EOF
$SECRETS
EOF
if [ "$REDACT_FAILED" -eq 1 ]; then
  echo 'Failed to redact secrets from log file'
  fail_safe
fi
if ! mv "${LOG_FILE}.tmp" "$LOG_FILE"; then
  echo 'Failed to replace log file with redacted version'
  fail_safe
fi
echo 'Log sanitization complete'
