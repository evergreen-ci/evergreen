package model

// GeneratorOptions hold all options common to all generator types,
// and are used in the configuration of generator functions and their
// dependency relationships.
type GeneratorOptions struct {
	JobID     string
	DependsOn []string
	NS        Namespace
	Query     map[string]interface{}
}
