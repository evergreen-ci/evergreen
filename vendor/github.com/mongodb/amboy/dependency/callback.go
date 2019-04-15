package dependency

const checkTypeName = "check"

type CheckFunc func([]string) State

type checkManager struct {
	CheckName string   `bson:"function_name" json:"function_name" yaml:"function_name"`
	T         TypeInfo `bson:"type" json:"type" yaml:"type"`
	JobEdges  `bson:"edges" json:"edges" yaml:"edges"`
}

// NewCheckManager creates a new check manager that will call the
// registered Check function matching that name. If no such function
// exists, then the manager is Unresolved.
func NewCheckManager(name string) Manager {
	m := makeCheckManager()
	m.CheckName = name
	return m
}

func makeCheckManager() *checkManager {
	return &checkManager{
		JobEdges: NewJobEdges(),
		T: TypeInfo{
			Version: 1,
			Name:    checkTypeName,
		},
	}

}

// Type returns the TypeInfo structure to satisfy the Manager
// interface.
func (d *checkManager) Type() TypeInfo { return d.T }

// State returns a state constant that can be used to determine if a
// dependency is satisfied.
func (d *checkManager) State() State {
	factory, err := GetCheckFactory(d.CheckName)
	if err != nil {
		return Unresolved
	}

	check := factory()

	return check(d.Edges())
}
