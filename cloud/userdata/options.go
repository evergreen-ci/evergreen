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

	// Persist indicates that the user data should run on every reboot rather
	// than just once. This is only used on Windows.
	Persist bool
}

// Validate verifies that the options are valid and sets defaults for
// closing tag if none is specified.
func (u *Options) Validate() error {
	catcher := grip.NewBasicCatcher()
	if u.Directive == "" {
		catcher.New("user data is missing directive")
	} else {
		var validDirective bool
		for _, directive := range Directives() {
			if strings.HasPrefix(string(u.Directive), string(directive)) {
				validDirective = true
				break
			}
		}
		catcher.ErrorfWhen(!validDirective, "directive '%s' is invalid", u.Directive)
	}

	catcher.ErrorfWhen(!u.Directive.CanPersist() && u.Persist,
		"cannot specify persisted user data with directive '%s'", u.Directive)

	return catcher.Resolve()
}
