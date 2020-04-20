package userdata

// Directive represents a marker that starts user data and indicates its
// type.
type Directive string

const (
	ShellScript   Directive = "#!"
	Include       Directive = "#include"
	CloudConfig   Directive = "#cloud-config"
	UpstartJob    Directive = "#upstart-job"
	CloudBoothook Directive = "#cloud-boothook"
	PartHandler   Directive = "#part-handler"
	// Windows-only types
	PowerShellScript Directive = "<powershell>"
	BatchScript      Directive = "<script>"
)

func Directives() []Directive {
	return []Directive{
		ShellScript,
		Include,
		CloudConfig,
		UpstartJob,
		CloudBoothook,
		PartHandler,
		PowerShellScript,
		BatchScript,
	}
}

// DirectiveToContentType maps a cloud-init directive to its MIME content type.
func DirectiveToContentType() map[Directive]string {
	return map[Directive]string{
		ShellScript:      "text/x-shellscript",
		Include:          "text/x-include-url",
		CloudConfig:      "text/cloud-config",
		UpstartJob:       "text/upstart-job",
		CloudBoothook:    "text/cloud-boothook",
		PartHandler:      "text/part-handler",
		PowerShellScript: "text/x-shellscript",
		BatchScript:      "text/x-shellscript",
	}
}

func (d Directive) NeedsClosingTag() bool {
	return d.ClosingTag() != ""
}

func (d Directive) ClosingTag() ClosingTag {
	switch d {
	case PowerShellScript:
		return PowerShellScriptClosingTag
	case BatchScript:
		return BatchScriptClosingTag
	}
	return ""
}

func NewShellScriptDirective(program string) Directive {
	return Directive(string(ShellScript) + program)
}
