//go:build !windows

package launch

// Tab colours are a Windows Terminal feature; elsewhere the hook does nothing.
func writeToClaudeConsole(string) {}
