package userdata

import (
	"strings"

	"github.com/pkg/errors"
)

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
func DirectiveToContentType(d Directive) (string, error) {
	for directive, contentType := range map[Directive]string{
		ShellScript:      "text/x-shellscript",
		Include:          "text/x-include-url",
		CloudConfig:      "text/cloud-config",
		UpstartJob:       "text/upstart-job",
		CloudBoothook:    "text/cloud-boothook",
		PartHandler:      "text/part-handler",
		PowerShellScript: "text/x-shellscript",
		BatchScript:      "text/x-shellscript",
	} {
		if strings.HasPrefix(string(d), string(directive)) {
			return contentType, nil
		}
	}
	return "", errors.Errorf("unrecognized cloud-init directive '%s'", d)
}

// NeedsClosingTag returns whether or not this directive must be closed.
func (d Directive) NeedsClosingTag() bool {
	return d.ClosingTag() != ""
}

// CanPersist returns whether or not this directive is compatible with
// persisting user data (i.e. running it on every boot). This is relevant to
// Windows only.
func (d Directive) CanPersist() bool {
	return d.ClosingTag() == PowerShellScriptClosingTag || d.ClosingTag() == BatchScriptClosingTag
}

// ClosingTag returns the closing tag that marks the end of the directive.
func (d Directive) ClosingTag() ClosingTag {
	switch d {
	case PowerShellScript:
		return PowerShellScriptClosingTag
	case BatchScript:
		return BatchScriptClosingTag
	}
	return ""
}
