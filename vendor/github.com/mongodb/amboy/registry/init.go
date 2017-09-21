package registry

import "github.com/mongodb/amboy/dependency"

// private initialization of the package-level registries.

var amboyRegistry *typeRegistry

func init() {
	amboyRegistry = newTypeRegistry()

	AddDependencyType(dependency.AlwaysRun, alwaysDependencyFactory)
	AddDependencyType(dependency.LocalFileRelationship, localFileDependencyFactory)
	AddDependencyType(dependency.Create, createsFileDependencyFactory)
}

func alwaysDependencyFactory() dependency.Manager {
	return dependency.NewAlways()
}

func localFileDependencyFactory() dependency.Manager {
	return dependency.NewLocalFileInstance()
}

func createsFileDependencyFactory() dependency.Manager {
	return dependency.NewCreatesFileInstance()
}
