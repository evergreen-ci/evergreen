package util

// PowershellQuotedString returns a string PowerShell which, when interpreted by
// PowerShell, all quotation marks in s are treated as literal quotation marks.
func PowershellQuotedString(s string) string {
	return "@'\n" + s + "\n'@"
}
