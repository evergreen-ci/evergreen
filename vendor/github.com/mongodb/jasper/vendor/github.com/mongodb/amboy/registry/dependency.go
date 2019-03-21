package registry

import "github.com/mongodb/amboy/dependency"

// AddDependencyType registers a new dependency.Manager factories.
func AddDependencyType(name string, f dependency.ManagerFactory) {
	dependency.RegisterManager(name, f)
}

// GetDependencyFactory returns a dependency.Manager factory function
// from the registry based on the name produced. If the name does not
// exist, then the error value is non-nil.
func GetDependencyFactory(name string) (dependency.ManagerFactory, error) {
	return dependency.GetManagerFactory(name)
}

// AddCheckType registers a callback function used in the
// production of some dependencies
func AddCheckType(name string, f dependency.CheckFactory) {
	dependency.RegisterCheck(name, f)
}

// GetCheckFactory returns a callback function factory for use in
// dependencies
func GetCheckFactory(name string) (dependency.CheckFactory, error) {
	return dependency.GetCheckFactory(name)
}
