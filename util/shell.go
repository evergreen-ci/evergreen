package util

import "strings"

// PowerShellQuotedString returns a string PowerShell which, when interpreted by
// PowerShell, all quotation marks in s are treated as literal quotation marks.
func PowerShellQuotedString(s string) string {
	return "@'\n" + strings.Replace(strings.Replace(s, `\`, `\\`, -1), `"`, `""`, -1) + "\n'@"
}

// ShellQuote returns s quoted for safe use as a single literal argument in a
// POSIX shell command.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
