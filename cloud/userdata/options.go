package userdata

import "github.com/mongodb/grip"

type Options struct {
	// Directive is the marker at the start of user data that determines the
	// type of user data.
	Directive Directive
	// Content is the content of the user data itself, excluding metadata such
	// as the directive and tags.
	Content string
	// ClosingTag is the marker that ends user data. This is only used on
	// Windows.
	ClosingTag ClosingTag
	// Persist indicates that the script should
	Persist bool
}

func (u *Options) Validate() error {
	catcher := grip.NewBasicCatcher()
	if u.Directive == "" {
		catcher.New("user data is missing directive")
	} else {
		var validDirective bool
		for _, directive := range Directives() {
			if u.Directive == directive {
				validDirective = true
				break
			}
		}
		catcher.ErrorfWhen(!validDirective, "directive '%s' is invalid", u.Directive)
	}
	catcher.ErrorfWhen(NeedsClosingTag(u.Directive) && u.ClosingTag != ClosingTagFor(u.Directive),
		"directive '%s' needs closing tag '%s' but actual closing tag is '%s'",
		u.Directive, ClosingTagFor(u.Directive), u.ClosingTag)
	return catcher.Resolve()
}

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
		// Windows-only types,
		PowerShellScript,
		BatchScript,
	}
}

// DirectiveToContentType maps a cloud-init directive to its MIME content type.
func DirectiveToContentType() map[Directive]string {
	return map[Directive]string{
		ShellScript:   "text/x-shellscript",
		Include:       "text/x-include-url",
		CloudConfig:   "text/cloud-config",
		UpstartJob:    "text/upstart-job",
		CloudBoothook: "text/cloud-boothook",
		PartHandler:   "text/part-handler",
		// Windows-only types
		PowerShellScript: "text/x-shellscript",
		BatchScript:      "text/x-shellscript",
	}
}

// ClosingTag represents a marker that ends user data.
type ClosingTag string

const (
	PowerShellScriptClosingTag ClosingTag = "</powershell>"
	BatchScriptClosingTag      ClosingTag = "</script>"
)

// closingTags returns all cloud-init closing tags for directives.
func ClosingTags() []ClosingTag {
	return []ClosingTag{
		PowerShellScriptClosingTag,
		BatchScriptClosingTag,
	}
}

// needsClosingTag returns whether the given user data directive needs to be
// closed or not.
func NeedsClosingTag(directive Directive) bool {
	return ClosingTagFor(directive) != ""
}

// ClosingTagFor returns the required closing tag for the given directive.
func ClosingTagFor(directive Directive) ClosingTag {
	switch directive {
	case PowerShellScript:
		return PowerShellScriptClosingTag
	case BatchScript:
		return BatchScriptClosingTag
	}
	return ""
}
