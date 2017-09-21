package mock

import (
	"encoding/json"
	"fmt"
)

// DependencyNetwork provides a mock implementation of the
// anser.DependencyNetworker interface. This implementation, unlike
// the default one, does no deduplicate the dependency.
type DependencyNetwork struct {
	Graph         map[string][]string
	Groups        map[string][]string
	ValidateError error
}

func NewDependencyNetwork() *DependencyNetwork {
	return &DependencyNetwork{
		Graph:  make(map[string][]string),
		Groups: make(map[string][]string),
	}
}

func (n *DependencyNetwork) Add(name string, deps []string) {
	if _, ok := n.Graph[name]; !ok {
		n.Graph[name] = []string{}
	}

	n.Graph[name] = append(n.Graph[name], deps...)
}

func (n *DependencyNetwork) Resolve(name string) []string { return n.Graph[name] }
func (n *DependencyNetwork) All() []string {
	out := []string{}

	for _, edges := range n.Graph {
		out = append(out, edges...)
	}

	return out
}

func (n *DependencyNetwork) Network() map[string][]string { return n.Graph }
func (n *DependencyNetwork) Validate() error              { return n.ValidateError }
func (n *DependencyNetwork) AddGroup(name string, group []string) {
	if _, ok := n.Graph[name]; !ok {
		n.Graph[name] = []string{}
	}

	n.Graph[name] = append(n.Graph[name], group...)

}
func (n *DependencyNetwork) GetGroup(name string) []string {
	out := []string{}

	for group := range n.Groups {
		out = append(out, group)
	}

	return out
}

func (n *DependencyNetwork) MarshalJSON() ([]byte, error) { return json.Marshal(n.Network()) }
func (n *DependencyNetwork) String() string               { return fmt.Sprintf("%v", n.Network()) }
