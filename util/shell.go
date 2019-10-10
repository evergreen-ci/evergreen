package util

import "strings"

// PowerShellQuotedString returns a string PowerShell which, when interpreted by
// PowerShell, all quotation marks in s are treated as literal quotation marks.
func PowerShellQuotedString(s string) string {
	return "@'\n" + strings.Replace(strings.Replace(s, `\`, `\\`, -1), `"`, `""`, -1) + "\n'@"
}
