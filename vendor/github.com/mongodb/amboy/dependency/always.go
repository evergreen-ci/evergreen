/*
Package dependency contains the Manager interface, along with several
implementations for different kinds of dependency checks.
*/
package dependency

// Always is a DependencyManager implementation that always reports
// that the task is ready to run.
type Always struct {
	ShouldRebuild bool     `json:"should_rebuild" bson:"should_rebuild" yaml:"should_rebuild"`
	T             TypeInfo `json:"type" bson:"type" yaml:"type"`
	*JobEdges
}

// NewAlways creates a DependencyManager object that always
// returns the "Ready" indicating that all dependency requirements
// are met and that the target is required.
func NewAlways() *Always {
	return &Always{
		ShouldRebuild: true,
		JobEdges:      NewJobEdges(),
		T: TypeInfo{
			Name:    AlwaysRun,
			Version: 0,
		},
	}
}

// State always returns, for Always, the "Ready" state.
func (d *Always) State() State {
	return Ready
}

// Type returns a DependencyInterchange object to assist in
// unmarshalling dependency objects.
func (d *Always) Type() TypeInfo {
	return d.T
}
