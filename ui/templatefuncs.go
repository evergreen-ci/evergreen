package ui

import (
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type FuncOptions struct {
	WebHome string
	IsProd  bool
	Router  *mux.Router
}

type MutableVar struct {
	Value interface{}
}

func (self *MutableVar) Get() interface{} {
	return self.Value
}

func (self *MutableVar) Set(v interface{}) interface{} {
	self.Value = v
	return ""
}

func MakeTemplateFuncs(fo FuncOptions) (template.FuncMap, error) {
	r := template.FuncMap{
		"Gravatar": func(email string) string {
			h := md5.New()
			io.WriteString(h, email)
			return fmt.Sprintf("http://www.gravatar.com/avatar/%x?s=50", h.Sum(nil))
		},

		"DateFormat": func(when time.Time, layout string, timezone string) string {
			if len(timezone) == 0 {
				timezone = "America/New_York"
			}
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				return util.DateAsString(when, layout)
			}

			whenTZ := when.In(loc)
			return util.DateAsString(whenTZ, layout)
		},

		// REMOVE: only used in patch page with ng-init. should be killed off.
		"Json": func(obj interface{}) (string, error) {
			v, err := json.Marshal(obj)
			if err != nil {
				return "", err
			}
			uninterpolateLeft := strings.Replace(string(v), "[[", "&#91;&#91;", -1)
			uninterpolateRight := strings.Replace(uninterpolateLeft, "]]", "&#93;&#93;", -1)
			return uninterpolateRight, nil
		},
		"Static": func(filetype, filename string) string {
			return fmt.Sprintf("/static/%s/%s", filetype, filename)
		},

		// Strip out ANSI color sequences in cases where it doesn't make sense to include
		// them, e.g. raw task logs
		"RemoveANSI": func(line string) string {
			re, err := regexp.Compile("\x1B\\[([0-9]{1,2}(;[0-9]{1,2})?)?[m|K]")
			if err != nil {
				return ""
			}
			return re.ReplaceAllString(line, "")
		},

		"IsProd": func() bool {
			return fo.IsProd
		},

		"GetTimezone": func(u *model.DBUser) string {
			if u != nil && u.Settings.Timezone != "" {
				return u.Settings.Timezone
			}
			return "America/New_York"
		},

		"MutableVar": func() interface{} {
			return &MutableVar{""}
		},
		"Trunc": func(s string, n int) string {
			if n > len(s) {
				return s
			}
			return s[0:n]
		},

		"UrlFor": func(name string, pairs ...interface{}) (*url.URL, error) {
			size := len(pairs)
			strPairs := make([]string, size, size)
			for i := 0; i < size; i++ {
				if v, ok := pairs[i].(string); ok {
					strPairs[i] = v
				} else {
					strPairs[i] = fmt.Sprint(pairs[i])
				}
			}

			route := fo.Router.Get(name)
			if route == nil {
				return nil, fmt.Errorf("UrlFor: can't find a route named %v", name)
			}

			return route.URL(strPairs...)
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
