package parsley

import (
	"reflect"
	"regexp"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
)

// Settings represents settings that can be configured for the Parsley log viewer.
type Settings struct {
	SectionsEnabled          *bool `bson:"sections_enabled,omitempty" json:"sections_enabled,omitempty"`
	JumpToFailingLineEnabled *bool `bson:"jump_to_failing_line_enabled,omitempty" json:"jump_to_failing_line_enabled,omitempty"`
}

// MergeExistingParsleySettings returns the merged result of oldSettings and newSettings.
// newSettings can be a subset of fields, so we populate the unset fields using the
// user's oldSettings.
func MergeExistingParsleySettings(oldSettings Settings, newSettings Settings) Settings {
	reflectOldSettings := reflect.ValueOf(&oldSettings).Elem()
	reflectNewSettings := reflect.ValueOf(&newSettings).Elem()
	util.RecursivelySetUndefinedFields(reflectNewSettings, reflectOldSettings)
	return newSettings
}

// Filter represents a filter for the Parsley log viewer. Parsley filters can be defined at
// the project-level and at the user-level.
type Filter struct {
	Description   string `bson:"description,omitempty" json:"description,omitempty"`
	Expression    string `bson:"expression" json:"expression"`
	CaseSensitive bool   `bson:"case_sensitive" json:"case_sensitive"`
	ExactMatch    bool   `bson:"exact_match" json:"exact_match"`
}

func (p Filter) validate() error {
	catcher := grip.NewSimpleCatcher()
	catcher.NewWhen(p.Expression == "", "filter expression must be non-empty")

	if _, regexErr := regexp.Compile(p.Expression); regexErr != nil {
		catcher.Wrapf(regexErr, "filter expression '%s' is invalid regexp", p.Expression)
	}

	return catcher.Resolve()
}

// ValidateFilters checks that there are no duplicate filters. It also validates each individual
// filter.
func ValidateFilters(filters []Filter) error {
	catcher := grip.NewBasicCatcher()

	filtersSet := make(map[Filter]bool)
	for _, f := range filters {
		if filtersSet[f] {
			catcher.Errorf("duplicate filter with expression '%s'", f.Expression)
		}
		filtersSet[f] = true
		catcher.Add(f.validate())
	}

	return catcher.Resolve()
}
