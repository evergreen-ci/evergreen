package mock

import (
	"github.com/mongodb/amboy/dependency"
)

type DependencyManager struct {
	Name       string
	Query      map[string]interface{}
	StateValue dependency.State
	T          dependency.TypeInfo
	*dependency.JobEdges
}

func (d *DependencyManager) Type() dependency.TypeInfo { return d.T }
func (d *DependencyManager) State() dependency.State   { return d.StateValue }
