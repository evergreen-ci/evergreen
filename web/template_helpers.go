package web

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/tychoish/grip/slogger"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
)

type timePeriod struct {
	secs      int
	unit      string
	units     string
	unitShort string
}

var Chunks = []timePeriod{
	{60 * 60 * 24, "day", "days", "d"},
	{60 * 60, "hour", "hours", "h"},
	{60, "min", "min", "m"},
	{1, "sec", "sec", "s"},
}

// Because Go's templating language for some reason doesn't allow assignments,
// use this to get around it.
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

func convertToTimezone(when time.Time, timezone string, layout string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		evergreen.Logger.Errorf(slogger.WARN, "Could not load location from timezone %v: %v", timezone, err)
		return when.Format(layout)
	}

	whenTZ := when.In(loc)
	return whenTZ.Format(layout)
}

func humanTimeDiff(sinceSecs int) []string {
	var i, count int
	var period timePeriod
	for i, period = range Chunks {
		count = sinceSecs / period.secs
		if count != 0 {
			break
		}
	}
	returnVal := make([]string, 0, 5)
	returnVal = append(returnVal, fmt.Sprintf("%d%s", count, period.unitShort))
	for i+1 < len(Chunks) {
		period2 := Chunks[i+1]
		leftover := sinceSecs - (period.secs * count)
		count2 := leftover / period2.secs
		if count2 != 0 {
			returnVal = append(returnVal, fmt.Sprintf("%d%s", count, period2.unitShort))
		}
		i++
	}
	return returnVal
}

//Create function Mappings - Add functions here to
//make them usable in the template language
func MakeCommonFunctionMap(settings *evergreen.Settings) (template.FuncMap,
	error) {
	funcs := map[string]interface{}{}

	//Equals function
	funcs["Eq"] = reflect.DeepEqual

	//Greater than function, with an optional threshold
	funcs["Gte"] = func(a, b, threshold int) bool {
		return a+threshold >= b
	}

	//Convenience function for ternary operator in templates
	// condition ? iftrue : otherwise
	funcs["Tern"] = func(condition bool, iftrue interface{}, otherwise interface{}) interface{} {
		if condition {
			return iftrue
		}
		return otherwise
	}

	// Unescape HTML. Be very careful that you don't pass any user input through
	// this, that would be an XSS vulnerability.
	funcs["Unescape"] = func(s string) interface{} {
		return template.HTML(s)
	}

	// return the base name for a file
	funcs["Basename"] = func(str string) string {
		lastSlash := strings.LastIndex(str, "/")
		if lastSlash == -1 || lastSlash == len(str)-1 {
			// try to find the index using windows-style filesystem separators
			lastSlash = strings.LastIndex(str, "\\")
			if lastSlash == -1 || lastSlash == len(str)-1 {
				return str
			}
		}
		return str[lastSlash+1:]
	}

	// Get 50x50 Gravatar profile pic URL for given email
	funcs["Gravatar"] = func(email string) string {
		h := md5.New()
		io.WriteString(h, email)

		return fmt.Sprintf("http://www.gravatar.com/avatar/%x?s=50", h.Sum(nil))
	}

	// jsonifying
	funcs["Json"] = func(obj interface{}) (string, error) {
		v, err := json.Marshal(obj)
		if err != nil {
			return "", err
		}
		uninterpolateLeft := strings.Replace(string(v), "[[", "&#91;&#91;", -1)
		uninterpolateRight := strings.Replace(uninterpolateLeft, "]]", "&#93;&#93;", -1)
		return uninterpolateRight, nil
	}

	//Truncate a string to the desired length.
	funcs["Trunc"] = util.Truncate

	funcs["IsProd"] = func() bool {
		return settings.IsProd
	}

	/* Unpleasant hack to make Go's templating language support assignments */
	funcs["MutableVar"] = func() interface{} {
		return &MutableVar{""}
	}

	//A map of systemwide globals, set up only once, which can be accessed via
	//template function for usage on the front-end.
	GLOBALS := make(map[string]string)
	GLOBALS["revision"] = "none" //evergreen.GetCurrentRevision()
	GLOBALS["uiUrl"] = settings.Ui.Url
	funcs["Global"] = func(key string) string {
		val, present := GLOBALS[key]
		if !present {
			return ""
		} else {
			return val
		}
	}

	// Remove ANSI color sequences in cases where it doesn't make sense to include
	// them, e.g. raw task logs
	funcs["RemoveANSI"] = func(line string) string {
		re, err := regexp.Compile("\x1B\\[([0-9]{1,2}(;[0-9]{1,2})?)?[m|K]")
		if err != nil {
			return ""
		}
		return re.ReplaceAllString(line, "")
	}

	return funcs, nil
}
