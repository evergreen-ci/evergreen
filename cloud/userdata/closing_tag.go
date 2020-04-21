package userdata

// ClosingTag represents a marker that ends user data.
type ClosingTag string

const (
	PowerShellScriptClosingTag ClosingTag = "</powershell>"
	BatchScriptClosingTag      ClosingTag = "</script>"
)

// ClosingTags returns all cloud-init closing tags for directives.
func ClosingTags() []ClosingTag {
	return []ClosingTag{
		PowerShellScriptClosingTag,
		BatchScriptClosingTag,
	}
}
