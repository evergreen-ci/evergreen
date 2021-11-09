package anser

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/tychoish/tarjan"
)

func newDependencyNetwork() model.DependencyNetworker {
	return &dependencyNetwork{
		network: make(map[string]map[string]struct{}),
		group:   make(map[string]map[string]struct{}),
	}
}

type dependencyNetwork struct {
	network map[string]map[string]struct{}
	group   map[string]map[string]struct{}
	mu      sync.RWMutex
}

func (n *dependencyNetwork) Add(name string, deps []string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	depSet, ok := n.network[name]
	if !ok {
		depSet = make(map[string]struct{})
		n.network[name] = depSet
	}

	for _, d := range deps {
		depSet[d] = struct{}{}
	}
}

func (n *dependencyNetwork) Resolve(name string) []string {
	out := []string{}

	n.mu.RLock()
	defer n.mu.RUnlock()

	edges, ok := n.network[name]
	if !ok {
		return out
	}

	for e := range edges {
		out = append(out, e)
	}

	return out
}

func (n *dependencyNetwork) All() []string {
	out := []string{}

	n.mu.RLock()
	defer n.mu.RUnlock()
	for name := range n.network {
		out = append(out, name)
	}

	return out
}

func (n *dependencyNetwork) Network() map[string][]string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return n.getNetworkUnsafe()
}

// this is implemented separately from network so we can use it in
// validation and have a sane locking strategy.
func (n *dependencyNetwork) getNetworkUnsafe() map[string][]string {
	out := make(map[string][]string)

	for node, edges := range n.network {
		deps := []string{}
		for e := range edges {
			deps = append(deps, e)
		}
		out[node] = deps
	}
	return out
}

func (n *dependencyNetwork) Validate() error {
	dependencies := make(map[string]struct{})
	catcher := grip.NewCatcher()

	n.mu.RLock()
	defer n.mu.RUnlock()

	graph := n.getNetworkUnsafe()
	for _, edges := range graph {
		for _, id := range edges {
			dependencies[id] = struct{}{}
		}
	}

	for id := range dependencies {
		if _, ok := n.network[id]; !ok {
			catcher.Add(fmt.Errorf("dependency %s is not defined", id))
		}
	}

	for _, group := range tarjan.Connections(graph) {
		if len(group) > 1 {
			catcher.Add(fmt.Errorf("cycle detected between nodes: [%s]",
				strings.Join(group, ", ")))
		}
	}

	return catcher.Resolve()
}

func (n *dependencyNetwork) AddGroup(name string, group []string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	groupSet, ok := n.group[name]
	if !ok {
		groupSet = make(map[string]struct{})
		n.group[name] = groupSet
	}

	for _, g := range group {
		groupSet[g] = struct{}{}
	}
}

func (n *dependencyNetwork) GetGroup(name string) []string {
	out := []string{}

	n.mu.RLock()
	defer n.mu.RUnlock()

	group, ok := n.group[name]
	if !ok {
		return out
	}

	for g := range group {
		out = append(out, g)
	}

	return out
}

//////////////////////////////////////////
//
// Output Formats

func (n *dependencyNetwork) MarshalJSON() ([]byte, error) { return json.Marshal(n.Network()) }
func (n *dependencyNetwork) String() string               { return fmt.Sprintf("%v", n.Network()) }
