package model

import (
	"encoding/json"
	"fmt"
)

// DependencyNetworker provides answers to questions about the
// dependencies of a task and is available white generating
// migrations. Methods should do nothing in particular
//
// Implementations should be mutable and thread-safe.
//
// The DependencyNetworker interface definition is in the model
// package because it has no external dependencies, and placing it in
// other packages would lead to cycles. The default implementation
// is in the top level package.
type DependencyNetworker interface {
	// Add inserts a list of dependencies for a given item. If the
	// slice of dependencies is empty, Add is a noop. Furthermore,
	// the Add method provides no validation, and will do nothing
	// to prevent cycles or broken dependencies.
	Add(string, []string)

	// Resolve, returns all of the dependencies for the specified task.
	Resolve(string) []string

	// All returns a list of all tasks that have registered
	// dependencies.
	All() []string

	// Network returns the dependency graph for all registered
	// tasks as a mapping of task IDs to the IDs of its
	// dependencies.
	Network() map[string][]string

	// Validate returns errors if there are either dependencies
	// specified that do not have tasks available *or* if there
	// are dependency cycles.
	Validate() error

	// AddGroup and GetGroup set and return the lists of tasks
	// that belong to a specific task group. Unlike the specific
	// task dependency setters.
	AddGroup(string, []string)
	GetGroup(string) []string

	// For introspection and convince, DependencyNetworker
	// composes implementations of common interfaces.
	fmt.Stringer
	json.Marshaler
}
