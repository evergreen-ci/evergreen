package model

// GeneratorOptions hold all options common to all generator types,
// and are used in the configuration of generator functions and their
// dependency relationships.
type GeneratorOptions struct {
	JobID     string                 `bson:"_id" json:"id" yaml:"id"`
	DependsOn []string               `bson:"dependencies" json:"dependencies" yaml:"dependencies"`
	NS        Namespace              `bson:"namespace" json:"namespace" yaml:"namespace"`
	Query     map[string]interface{} `bson:"query" json:"query" yaml:"query"`
	Limit     int                    `bson:"limit" json:"limit" yaml:"limit"`
}

func (o GeneratorOptions) IsValid() bool {
	if !o.NS.IsValid() {
		return false
	}

	if o.JobID == "" {
		return false
	}

	if o.Limit < 0 {
		return false
	}

	// it might be reasonable to require that there be a query,
	// but you could use a generator to modify every document in the
	// collection, so we'll leave it off for now.

	return true
}
