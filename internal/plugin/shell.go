package plugin

import "strings"

// ShellQuote escapes single quotes in s for safe embedding in single-quoted
// shell strings. The result is the inner content, not the outer quotes.
func ShellQuote(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}
