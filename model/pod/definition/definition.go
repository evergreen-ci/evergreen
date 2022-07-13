package definition

// PodDefinition represents a template definition for a pod kept in external
// storage.
// kim: TOOD: add standard DB fns and BSON keys.
type PodDefinition struct {
	// ID is the unique identifier for this document.
	ID string
	// Digest is the value for the hashed pod definition parameters.
	Digest string
	// ExternalID is the identifier for the template definition in external
	// storage.
	ExternalID string
}
