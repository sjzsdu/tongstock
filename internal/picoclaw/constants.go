package picoclaw

import "strings"

const (
	DefaultAgentID = "main"
	defaultAgentID = DefaultAgentID
	defaultSession = "tongstock:default"
	defaultModel   = "main"
)

func str(s string) string {
	return strings.TrimSpace(s)
}
