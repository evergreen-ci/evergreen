package model

import "github.com/pkg/errors"

// ReportFormat is an output format for reporting files.
type ReportFormat string

const (
	Artifact      ReportFormat = "artifact"
	EvergreenJSON ReportFormat = "evg-json"
	GoTest        ReportFormat = "gotest"
	XUnit         ReportFormat = "xunit"
)

// Validate checks that the report format is recognized.
func (r ReportFormat) Validate() error {
	switch r {
	case Artifact, EvergreenJSON, GoTest, XUnit:
		return nil
	default:
		return errors.Errorf("unrecognized format '%s'", r)
	}
}
