package dependency

// MockDependency implements the dependency.Manager interface, but
// provides the capability to mock out the behavior of the State()
// method.
type MockDependency struct {
	Response State
	T        TypeInfo
	JobEdges
}

// NewMock constructs a new mocked dependency object.
func NewMock() *MockDependency {
	return &MockDependency{
		T: TypeInfo{
			Name:    "mock",
			Version: 0,
		},
		JobEdges: NewJobEdges(),
	}
}

// State returns a state value derived from the Response field in the
// MockDependency struct.
func (d *MockDependency) State() State { return d.Response }

// Type returns the TypeInfo value for this dependency implementation.
func (d *MockDependency) Type() TypeInfo { return d.T }
