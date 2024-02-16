package parsley

import (
	"regexp"

	"github.com/mongodb/grip"
)

// ParsleyFilter represents a filter for the Parsley log viewer. Parsley filters can be defined at
// the project-level and at the user-level.
type ParsleyFilter struct {
	Expression    string `bson:"expression" json:"expression"`
	CaseSensitive bool   `bson:"case_sensitive" json:"case_sensitive"`
	ExactMatch    bool   `bson:"exact_match" json:"exact_match"`
}

func (p ParsleyFilter) validate() error {
	catcher := grip.NewSimpleCatcher()
	catcher.NewWhen(p.Expression == "", "filter expression must be non-empty")

	if _, regexErr := regexp.Compile(p.Expression); regexErr != nil {
		catcher.Wrapf(regexErr, "filter expression '%s' is invalid regexp", p.Expression)
	}

	return catcher.Resolve()
}

// ValidateParsleyFilters checks that there are no duplicate expressions among the Parsley filters. It also validates
// each individual Parsley filter.
func ValidateParsleyFilters(parsleyFilters []ParsleyFilter) error {
	catcher := grip.NewBasicCatcher()

	filtersSet := make(map[string]bool)
	for _, filter := range parsleyFilters {
		if filtersSet[filter.Expression] {
			catcher.Errorf("duplicate filter expression '%s'", filter.Expression)
		}
		filtersSet[filter.Expression] = true
		catcher.Add(filter.validate())
	}

	return catcher.Resolve()
}
