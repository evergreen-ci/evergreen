package service

import (
	"crypto/md5"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
)

const defaultHelpURL = "https://github.com/evergreen-ci/evergreen/wiki/How-To-Read-Evergreen"

// FuncOptions are global variables injected into our templating functions.
type TemplateFunctionOptions struct {
	WebHome  string
	HelpHome string
	IsProd   bool
}

// MutableVar is a setable variable for UI templates.
// This utility allows us to set and retrieve values, which
// is not a built in feature of Go HTML templates.
type MutableVar struct {
	Value interface{}
}

// Get returns the value of the variable.
func (self *MutableVar) Get() interface{} {
	return self.Value
}

// Set sets the value of the variable.
// Set returns the empty string so as to not pollute the resulting template.
func (self *MutableVar) Set(v interface{}) interface{} {
	self.Value = v
	return ""
}

// MakeTemplateFuncs creates and registers all of our built-in template functions.
func MakeTemplateFuncs(fo TemplateFunctionOptions, superUsers []string) (map[string]interface{}, error) {
	r := map[string]interface{}{
		// IsSuperUser returns true if the given user Id has super user privileges.
		"IsSuperUser": func(userName string) bool {
			return len(superUsers) == 0 || util.StringSliceContains(superUsers, userName)
		},
		// Gravatar returns a Gravatar URL for the given email string.
		"Gravatar": func(email string) string {
			h := md5.New()
			_, err := io.WriteString(h, email)
			grip.Warning(err)
			return fmt.Sprintf("http://www.gravatar.com/avatar/%x?s=50", h.Sum(nil))
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

		// RemoveANSI strips out ANSI color sequences in cases where it doesn't make sense to include
		// them, e.g. raw task logs
		"RemoveANSI": func(line string) string {
			re, err := regexp.Compile("\x1B\\[([0-9]{1,2}(;[0-9]{1,2})?)?[m|K]")
			if err != nil {
				return ""
			}
			return re.ReplaceAllString(line, "")
		},

		// Is Prod returns whether or not Evergreen is running in "production."
		// Currently this is only used to toggle the use of minified css files.
		"IsProd": func() bool {
			return fo.IsProd
		},

		// GetTimezone returns the timezone for a user.
		// Defaults to "New_York".
		"GetTimezone": func(u *user.DBUser) string {
			if u != nil && u.Settings.Timezone != "" {
				return u.Settings.Timezone
			}
			return "America/New_York"
		},

		// MutableVar creates an unset MutableVar.
		"MutableVar": func() interface{} {
			return &MutableVar{""}
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

	staticsMD5, err := DirectoryChecksum(fo.WebHome)
	if err != nil {
		return nil, err
	}

	r["StaticsMD5"] = func() string {
		return staticsMD5
	}
	return r, nil

}
