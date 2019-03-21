package registry

import "github.com/mongodb/amboy/dependency"

// this is is a mock dependency implementation that's here

type CheckTest struct {
	T dependency.TypeInfo `json:"type" bson:"type" yaml:"type"`
	dependency.JobEdges
}

func init() {
	AddDependencyType("test", func() dependency.Manager {
		return NewCheckTestDependency()
	})
}

// NewCheckTestDependency creates a DependencyManager object that always
// returns the "Passed" indicating that all dependency requirements
// are met and that ing the target is required.
func NewCheckTestDependency() *CheckTest {
	return &CheckTest{
		JobEdges: dependency.NewJobEdges(),
		T: dependency.TypeInfo{
			Name:    "test",
			Version: 0,
		},
	}
}

// State always returns, for CheckTest, the "Ready" state.
func (d *CheckTest) State() dependency.State {
	return dependency.Ready
}

// Type returns a DependencyInterchange object to assist in
// unmarshalling dependency objects.
func (d *CheckTest) Type() dependency.TypeInfo {
	return d.T
}
