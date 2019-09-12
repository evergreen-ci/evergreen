package service

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

const defaultHelpURL = "https://github.com/evergreen-ci/evergreen/wiki/How-To-Read-Evergreen"

// FuncOptions are global variables injected into our templating functions.
type TemplateFunctionOptions struct {
	WebHome  string
	HelpHome string
}

// MakeTemplateFuncs creates and registers all of our built-in template functions.
func MakeTemplateFuncs(fo TemplateFunctionOptions, superUsers []string) map[string]interface{} {
	r := map[string]interface{}{
		// IsSuperUser returns true if the given user Id has super user privileges.
		"IsSuperUser": func(userName string) bool {
			return len(superUsers) == 0 || util.StringSliceContains(superUsers, userName)
		},
		// DateFormat returns a time Formatted to the given layout and timezone.
		// If the timezone is unset, it defaults to "New_York."
		"DateFormat": func(when time.Time, layout string, timezone string) string {
			if len(timezone) == 0 {
				timezone = "America/New_York" // I ♥ NY
			}
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				return when.Format(layout)
			}

			whenTZ := when.In(loc)
			return whenTZ.Format(layout)
		},

		// Static returns a link to a static file.
		"Static": func(filetype, filename string) string {
			return fmt.Sprintf("/static/%s/%s", filetype, filename)
		},

		// GetTimezone returns the timezone for a user.
		// Defaults to "New_York".
		"GetTimezone": func(u gimlet.User) string {
			usr, ok := u.(*user.DBUser)
			if ok && usr != nil && usr.Settings.Timezone != "" {
				return usr.Settings.Timezone
			}
			return "America/New_York"
		},
		// Trunc cuts off a string to be n characters long.
		"Trunc": func(s string, n int) string {
			if n > len(s) {
				return s
			}
			return s[0:n]
		},

		// HelpUrl returns the address of the Evergreen help page,
		// if one is set.
		"HelpUrl": func() string {
			if fo.HelpHome != "" {
				return fo.HelpHome
			}
			return defaultHelpURL
		},
	}

	r["BuildRevision"] = func() string {
		return evergreen.BuildRevision
	}
	return r

}
