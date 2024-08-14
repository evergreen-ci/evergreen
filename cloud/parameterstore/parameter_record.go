package parameterstore

import (
	"time"
)

// parameterRecord stores metadata information about parameters. This never
// stores the value of the parameter itself.
// kim: TODO: consider not exporting this since it's internal.
type parameterRecord struct {
	// Name is the unique full path identifier for the parameter.
	Name string `bson:"_id" json:"_id"`
	// LastUpdated is the time the parameter was most recently updated.
	LastUpdated time.Time `bson:"last_updated" json:"last_updated"`
}
