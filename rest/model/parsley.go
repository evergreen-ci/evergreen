package model

import (
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/utility"
)

type APIParsleySettings struct {
	//SectionsEnabled describes whether to render task logs with sections.
	SectionsEnabled *bool `json:"sections_enabled"`
	// JumpToFailingLineEnabled describes whether to automatically scroll to the failing log line on initial page load.
	JumpToFailingLineEnabled *bool `json:"jump_to_failing_line_enabled"`
}

func (s *APIParsleySettings) BuildFromService(settings parsley.Settings) {
	// Sections should be enabled for all users by default.
	if settings.SectionsEnabled == nil {
		s.SectionsEnabled = utility.TruePtr()
	} else {
		s.SectionsEnabled = settings.SectionsEnabled
	}

	// Jump to failing log line should be enabled for all users by default.
	if settings.JumpToFailingLineEnabled == nil {
		s.JumpToFailingLineEnabled = utility.TruePtr()
	} else {
		s.JumpToFailingLineEnabled = settings.JumpToFailingLineEnabled
	}
}

func (s *APIParsleySettings) ToService() parsley.Settings {
	return parsley.Settings{
		SectionsEnabled:          s.SectionsEnabled,
		JumpToFailingLineEnabled: s.JumpToFailingLineEnabled,
	}
}

type APIParsleyFilter struct {
	// Description is a description for the filter.
	Description *string `json:"description"`
	// Expression is a regular expression representing the filter.
	Expression *string `json:"expression"`
	// CaseSensitive indicates whether the filter is case sensitive.
	CaseSensitive *bool `json:"case_sensitive"`
	// ExactMatch indicates whether the filter must be an exact match.
	ExactMatch *bool `json:"exact_match"`
}

func (t *APIParsleyFilter) ToService() parsley.Filter {
	return parsley.Filter{
		Description:   utility.FromStringPtr(t.Description),
		Expression:    utility.FromStringPtr(t.Expression),
		CaseSensitive: utility.FromBoolPtr(t.CaseSensitive),
		ExactMatch:    utility.FromBoolPtr(t.ExactMatch),
	}
}

func (t *APIParsleyFilter) BuildFromService(h parsley.Filter) {
	t.Description = utility.ToStringPtr(h.Description)
	t.Expression = utility.ToStringPtr(h.Expression)
	t.CaseSensitive = utility.ToBoolPtr(h.CaseSensitive)
	t.ExactMatch = utility.ToBoolPtr(h.ExactMatch)
}
