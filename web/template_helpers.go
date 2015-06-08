package web

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"html/template"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

type timePeriod struct {
	secs      int
	unit      string
	units     string
	unitShort string
}

var Chunks = []timePeriod{
	timePeriod{60 * 60 * 24, "day", "days", "d"},
	timePeriod{60 * 60, "hour", "hours", "h"},
	timePeriod{60, "min", "min", "m"},
	timePeriod{1, "sec", "sec", "s"},
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

// for sorting the repo names by how many projects each has
type sortableRepoNames struct {
	names       []string
	namesByRepo map[string][]model.ProjectRef
}

func (self *sortableRepoNames) Len() int {
	return len(self.names)
}

func (self *sortableRepoNames) Swap(i, j int) {
	self.names[i], self.names[j] = self.names[j], self.names[i]
}

// considers one repo to come before another (be "less" than) if it only has
// one project associated and the other has more.  otherwise, falls back to
// alphabetical order of the repo's name.
func (self *sortableRepoNames) Less(i, j int) bool {
	if len(self.namesByRepo[self.names[i]]) > 1 &&
		len(self.namesByRepo[self.names[j]]) == 1 {
		return false
	}
	if len(self.namesByRepo[self.names[j]]) > 1 &&
		len(self.namesByRepo[self.names[i]]) == 1 {
		return true
	}
	return (self.namesByRepo[self.names[i]][0].DisplayName <
		self.namesByRepo[self.names[j]][0].DisplayName)
}

// for sorting model.ProjectRefs
type sortableDisplayProjects []model.ProjectRef

func (self sortableDisplayProjects) Len() int {
	return len(self)
}
func (self sortableDisplayProjects) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}
func (self sortableDisplayProjects) Less(i, j int) bool {
	return self[i].DisplayName < self[j].DisplayName
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

	// get info about the projects, and sort by repo
	projectRefs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return nil, fmt.Errorf("Error getting info on tracked projects: %v", err)
	}

	// build the map of repo -> projects
	projectRefsByRepo := map[string][]model.ProjectRef{}
	publicProjectRefsByRepo := map[string][]model.ProjectRef{}
	for _, pRef := range projectRefs {
		if !pRef.Enabled {
			continue
		}
		if !pRef.Private {
			publicProjectRefsByRepo[pRef.Repo] = append(publicProjectRefsByRepo[pRef.Repo], pRef)
		}
		projectRefsByRepo[pRef.Repo] = append(projectRefsByRepo[pRef.Repo], pRef)
	}

	repoNames := []string{}
	for repoName, _ := range projectRefsByRepo {
		// sort the project display names alphabetically
		sort.Sort(sortableDisplayProjects(projectRefsByRepo[repoName]))
		// make a unique list of repo names
		repoNames = append(repoNames, repoName)
	}

	publicRepoNames := []string{}
	for repoName, _ := range publicProjectRefsByRepo {
		// sort the project display names alphabetically
		sort.Sort(sortableDisplayProjects(publicProjectRefsByRepo[repoName]))
		// make a unique list of public repo names
		publicRepoNames = append(publicRepoNames, repoName)
	}

	// return the project infos, sorted by repo
	funcs["ProjectNamesByRepo"] = func(user auth.User) map[string][]model.ProjectRef {
		if user != nil {
			return projectRefsByRepo
		}
		return publicProjectRefsByRepo
	}

	// sort the repo names based on info about how many projects are associated
	forSorting := &sortableRepoNames{
		names:       repoNames,
		namesByRepo: projectRefsByRepo,
	}
	sort.Sort(forSorting)
	funcs["SortedRepoNames"] = func(user auth.User) []string {
		if user != nil {
			return repoNames
		}
		return publicRepoNames
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
