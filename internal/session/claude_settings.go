package session

import (
	"os"
	"path/filepath"
)

// claudeSettings is the contents of .claude/settings.local.json dropped into
// each worktree. Hooks call back into the kanban API with the session's
// active state so the ticket badge in the UI reflects what claude is doing.
//
// The hook commands rely on KANBAN_SESSION_ID and KANBAN_API_URL being
// injected into the session's environment by the session manager. Each hook
// shells out to .claude/kanban-status.sh, which logs every attempt (and the
// resulting curl exit code / HTTP status) to $TMPDIR/kanban-status-hook.log
// so failures on host-mode runs are debuggable instead of silently swallowed.
const claudeSettings = `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PROJECT_DIR:-.}/.claude/kanban-status.sh\" working user_prompt_submit"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PROJECT_DIR:-.}/.claude/kanban-status.sh\" idle stop"
          }
        ]
      }
    ],
    "Notification": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PROJECT_DIR:-.}/.claude/kanban-status.sh\" awaiting_perm notification"
          }
        ]
      }
    ]
  }
}
`

// kanbanStatusScript posts a status update to the kanban API and appends a
// structured log line per invocation. Errors are tolerated (the agent must
// never block on a kanban outage) but they are recorded so we can tell the
// difference between "hook never fired", "env vars missing", "DNS failed",
// and "API rejected the payload" -- which is exactly the ambiguity that
// prompted this script in the first place.
const kanbanStatusScript = `#!/bin/sh
# Usage: kanban-status.sh <status> <event>
status="${1:-}"
event="${2:-}"
log="${KANBAN_STATUS_LOG:-${TMPDIR:-/tmp}/kanban-status-hook.log}"
ts="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u)"

# Best-effort: ignore failures so the hook never blocks claude.
{
  printf '[%s] event=%s status=%s session=%s api=%s\n' \
    "$ts" "$event" "$status" "${KANBAN_SESSION_ID:-<unset>}" "${KANBAN_API_URL:-<unset>}"
} >>"$log" 2>/dev/null

if [ -z "$KANBAN_SESSION_ID" ] || [ -z "$KANBAN_API_URL" ]; then
  printf '[%s]   skip: KANBAN_SESSION_ID or KANBAN_API_URL is empty (not running through kanban session manager?)\n' "$ts" >>"$log" 2>/dev/null
  exit 0
fi

url="$KANBAN_API_URL/api/sessions/$KANBAN_SESSION_ID/status"
body="{\"status\":\"$status\"}"

# -w prints curl's view of the result; -o /dev/null discards the response body.
# stderr captures connection failures (DNS, refused, timeout). 2>&1 routes both
# into the log so we get the full picture for any failure mode.
result="$(curl -sS -m 5 -o /dev/null \
  -w 'http_code=%{http_code} time=%{time_total}s url=%{url_effective}' \
  -X PATCH -H 'Content-Type: application/json' \
  -d "$body" "$url" 2>&1)"
rc=$?
printf '[%s]   curl exit=%s %s\n' "$ts" "$rc" "$result" >>"$log" 2>/dev/null
exit 0
`

// writeClaudeSettings writes .claude/settings.local.json into the worktree if
// it does not already exist, plus the helper kanban-status.sh script the
// hooks invoke. Existing settings.local.json is left alone so user-authored
// hook configuration is preserved; the helper script is overwritten on every
// call so bug fixes in the script roll out without manual cleanup.
func writeClaudeSettings(worktreePath string) error {
	dir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	scriptPath := filepath.Join(dir, "kanban-status.sh")
	if err := os.WriteFile(scriptPath, []byte(kanbanStatusScript), 0o755); err != nil {
		return err
	}
	settingsPath := filepath.Join(dir, "settings.local.json")
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	}
	return os.WriteFile(settingsPath, []byte(claudeSettings), 0o644)
}
