package plugin

import "strings"

// ShellQuote escapes single quotes in s for safe embedding in single-quoted
// shell strings. The result is the inner content, not the outer quotes.
func ShellQuote(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// ShellQuoteJoin single-quotes each argument and joins them with spaces,
// producing a shell-safe command string. Arguments with spaces, single quotes,
// or metacharacters are preserved as single tokens.
func ShellQuoteJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + ShellQuote(a) + "'"
	}
	return strings.Join(quoted, " ")
}
