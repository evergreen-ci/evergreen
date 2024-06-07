package userdata

// ClosingTag represents a marker that ends user data.
type ClosingTag string

const (
	PowerShellScriptClosingTag ClosingTag = "</powershell>"
	BatchScriptClosingTag      ClosingTag = "</script>"
)
