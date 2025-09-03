package model

import "github.com/evergreen-ci/evergreen"

// APIBetaFeatures is the API model for BetaFeatures.
type APIBetaFeatures struct {
	SpruceWaterfallEnabled bool `json:"spruce_waterfall_enabled"`
	ParsleyAIEnabled       bool `json:"parsley_ai_enabled"`
}

// BuildFromService converts from service level BetaFeatures to
// APIBetaFeatures.
func (a *APIBetaFeatures) BuildFromService(b evergreen.BetaFeatures) {
	a.SpruceWaterfallEnabled = b.SpruceWaterfallEnabled
	a.ParsleyAIEnabled = b.ParsleyAIEnabled
}

// ToService returns a service layer BetaFeatures using the data
// from APIBetaFeatures.
func (a *APIBetaFeatures) ToService() evergreen.BetaFeatures {
	return evergreen.BetaFeatures{
		SpruceWaterfallEnabled: a.SpruceWaterfallEnabled,
		ParsleyAIEnabled:       a.ParsleyAIEnabled,
	}
}
