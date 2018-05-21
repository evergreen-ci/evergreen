package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

type APIWaterfallData struct {
	Rows              []model.WaterfallRow     `json:"rows"`
	Versions          []model.WaterfallVersion `json:"versions"`
	TotalVersions     int                      `json:"total_versions"`      // total number of versions (for pagination)
	CurrentSkip       int                      `json:"current_skip"`        // number of versions skipped so far
	PreviousPageCount int                      `json:"previous_page_count"` // number of versions on previous page
	CurrentTime       int64                    `json:"current_time"`        // time used to calculate the eta of started task
}

func (apiWatefall *APIWaterfallData) BuildFromService(waterfallData interface{}) error {
	waterfallDatai, ok := waterfallData.(model.WaterfallData)

	if !ok {
		return errors.Errorf("incorrect type when fetching converting version type")
	}

	apiWatefall.Rows = waterfallDatai.Rows
	apiWatefall.Versions = waterfallDatai.Versions
	apiWatefall.TotalVersions = waterfallDatai.TotalVersions
	apiWatefall.CurrentSkip = waterfallDatai.CurrentSkip
	apiWatefall.PreviousPageCount = waterfallDatai.PreviousPageCount
	apiWatefall.CurrentTime = waterfallDatai.CurrentTime

	return nil
}

func (apiWatefall APIWaterfallData) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
