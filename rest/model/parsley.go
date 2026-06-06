package model

import (
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/utility"
)

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
