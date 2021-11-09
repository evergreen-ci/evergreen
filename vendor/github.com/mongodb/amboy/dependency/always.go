/*
Package dependency contains the Manager interface, along with several
implementations for different kinds of dependency checks.
*/
package dependency

const alwaysRunName = "always"

// Always is a DependencyManager implementation that always reports
// that the task is ready to run.
type alwaysManager struct {
	T TypeInfo `json:"type" bson:"type" yaml:"type"`
	JobEdges
}

// NewAlways creates a DependencyManager object that always
// returns the "Ready" indicating that all dependency requirements
// are met and that the target is required.
func NewAlways() Manager {
	return &alwaysManager{
		JobEdges: NewJobEdges(),
		T: TypeInfo{
			Name:    alwaysRunName,
			Version: 0,
		},
	}
}

// State always returns, for Always, the "Ready" state.
func (d *alwaysManager) State() State {
	return Ready
}

// Type returns a DependencyInterchange object to assist in
// unmarshalling dependency objects.
func (d *alwaysManager) Type() TypeInfo {
	return d.T
}
