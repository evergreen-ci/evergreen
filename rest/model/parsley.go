package model

import (
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/utility"
)

type APIParsleySettings struct {
	//SectionsEnabled describes whether to render task logs with sections.
	SectionsEnabled *bool `json:"sections_enabled"`
}

func (s *APIParsleySettings) BuildFromService(settings parsley.Settings) {
	// Sections should be enabled for all users by default.
	if settings.SectionsEnabled == nil {
		s.SectionsEnabled = utility.TruePtr()
	} else {
		s.SectionsEnabled = settings.SectionsEnabled
	}
}

func (s *APIParsleySettings) ToService() parsley.Settings {
	return parsley.Settings{
		SectionsEnabled: s.SectionsEnabled,
	}
}

type APIParsleyFilter struct {
	// Expression is a regular expression representing the filter.
	Expression *string `json:"expression"`
	// CaseSensitive indicates whether the filter is case sensitive.
	CaseSensitive *bool `json:"case_sensitive"`
	// ExactMatch indicates whether the filter must be an exact match.
	ExactMatch *bool `json:"exact_match"`
}

func (t *APIParsleyFilter) ToService() parsley.Filter {
	return parsley.Filter{
		Expression:    utility.FromStringPtr(t.Expression),
		CaseSensitive: utility.FromBoolPtr(t.CaseSensitive),
		ExactMatch:    utility.FromBoolPtr(t.ExactMatch),
	}
}

func (t *APIParsleyFilter) BuildFromService(h parsley.Filter) {
	t.Expression = utility.ToStringPtr(h.Expression)
	t.CaseSensitive = utility.ToBoolPtr(h.CaseSensitive)
	t.ExactMatch = utility.ToBoolPtr(h.ExactMatch)
}
