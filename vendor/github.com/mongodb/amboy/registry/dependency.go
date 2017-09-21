package registry

import "github.com/mongodb/amboy/dependency"

// DependencyFactory is a function that takes no arguments and returns
// a dependency.Manager interface. When implementing a new dependency
// type, also register a factory function with the DependencyFactory
// signature to facilitate serialization.
type DependencyFactory func() dependency.Manager

// AddDependencyType registers a new dependency.Manager factories.
func AddDependencyType(name string, f DependencyFactory) {
	amboyRegistry.registerDependencyType(name, f)
}

// GetDependencyFactory returns a dependency.Manager factory function
// from the registry based on the name produced. If the name does not
// exist, then the error value is non-nil.
func GetDependencyFactory(name string) (DependencyFactory, error) {
	return amboyRegistry.getDependencyFactory(name)
}
