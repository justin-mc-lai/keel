package agent

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/justin-mc-lai/agentdesk/internal/workspace"
)

// attachRe matches pi-style @path references.
var attachRe = regexp.MustCompile(`@([^\s@]+)`)

const maxAttachBytes = 64 * 1024

// ExpandAttachments replaces @relative/path with the file's contents.
//
// A reference that does not resolve inside the workspace, or does not exist, is
// left untouched: an email address or a @mention must not break a message.
func ExpandAttachments(ws *workspace.Workspace, text string) string {
	if ws == nil {
		return text
	}
	var blocks []string
	out := attachRe.ReplaceAllStringFunc(text, func(m string) string {
		rel := strings.TrimPrefix(m, "@")
		rel = strings.TrimRight(rel, ".,;:)")
		abs, err := ws.Resolve(rel)
		if err != nil {
			return m
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			return m
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			return m
		}
		body := string(raw)
		if len(body) > maxAttachBytes {
			body = body[:maxAttachBytes] + "\n[truncated]"
		}
		blocks = append(blocks, fmt.Sprintf("<attached path=%q>\n%s\n</attached>", rel, body))
		return m
	})
	if len(blocks) == 0 {
		return text
	}
	return strings.Join(blocks, "\n") + "\n\n" + out
}
