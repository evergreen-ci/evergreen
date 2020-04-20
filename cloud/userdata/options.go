package userdata

import (
	"strings"

	"github.com/mongodb/grip"
)

// Options represent parameters for generating user data.
type Options struct {
	// Directive is the marker at the start of user data that determines the
	// type of user data.
	Directive Directive
	// Content is the content of the user data itself, excluding metadata such
	// as the directive and tags.
	Content string

	// Windows-only options

	// ClosingTag is the marker that ends user data.
	ClosingTag ClosingTag
	// Persist indicates that the user data should run on every reboot rather
	// than just once.
	Persist bool
}

// ValidateAndDefault verifies that the options are valid and sets defaults for
// closing tag if none is specified.
func (u *Options) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	if u.Directive == "" {
		catcher.New("user data is missing directive")
	} else {
		var validDirective bool
		for _, directive := range Directives() {
			if u.Directive == directive {
				validDirective = true
				break
			} else if directive == ShellScript && strings.HasPrefix(string(u.Directive), directive) {
				// Shell scripts are a special case where we should allow any
				// string following "#!" since it will invoke the program.
				validDirective = true
				break
			}
		}
		catcher.ErrorfWhen(!validDirective, "directive '%s' is invalid", u.Directive)
	}

	needsClosingTag := u.Directive.NeedsClosingTag()
	requiredClosingTag := u.Directive.ClosingTag()

	if u.ClosingTag != "" {
		if !needsClosingTag {
			catcher.Errorf("directive '%s' should not have closing tag '%s'",
				u.Directive, u.ClosingTag)
		} else if u.ClosingTag != requiredClosingTag {
			catcher.Errorf("directive '%s' requires closing tag '%s' but actual closing tag is '%s'",
				u.Directive, requiredClosingTag, u.ClosingTag)
		}
	} else if needsClosingTag {
		u.ClosingTag = requiredClosingTag
	}

	return catcher.Resolve()
}
