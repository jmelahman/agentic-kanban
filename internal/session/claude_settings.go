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
// injected into the session container's environment by the session manager.
// Failures are swallowed so the agent never blocks on a kanban outage.
const claudeSettings = `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "curl -fsS -m 2 -X PATCH -H 'Content-Type: application/json' -d '{\"status\":\"working\"}' \"$KANBAN_API_URL/api/sessions/$KANBAN_SESSION_ID/status\" >/dev/null 2>&1 || true"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "curl -fsS -m 2 -X PATCH -H 'Content-Type: application/json' -d '{\"status\":\"idle\"}' \"$KANBAN_API_URL/api/sessions/$KANBAN_SESSION_ID/status\" >/dev/null 2>&1 || true"
          }
        ]
      }
    ],
    "Notification": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "curl -fsS -m 2 -X PATCH -H 'Content-Type: application/json' -d '{\"status\":\"awaiting_perm\"}' \"$KANBAN_API_URL/api/sessions/$KANBAN_SESSION_ID/status\" >/dev/null 2>&1 || true"
          }
        ]
      }
    ]
  }
}
`

// writeClaudeSettings writes .claude/settings.local.json into the worktree if
// it does not already exist. Existing files are left alone so user-authored
// hook configuration is preserved.
func writeClaudeSettings(worktreePath string) error {
	dir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.local.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(claudeSettings), 0o644)
}
