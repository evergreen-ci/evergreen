package parameterstore

import "time"

const Collection = "parameter_metadata"

// Parameter stores metadata about a parameter kept in Parameter Store. This
// should only store metadata about the parameter, not the value itself.
type parameterMetadata struct {
	// Name is the unique full path identifier the parameter.
	Name string `bson:"_id" json:"_id"`
	// LastUpdated is the time the parameter was most recently updated.
	LastUpdated time.Time `bson:"last_updated" json:"last_updated"`
}
