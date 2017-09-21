package dependency

// These values provide consistent ways of referring to the type names
// for dependencies.
const (
	// AlwaysRun dependencies are tasks that *always* run.
	AlwaysRun = "always"

	// Create dependencies run only if a file does *not* exist.
	Create = "create-file"

	// LocalFileRelationship dependencies compare a list of
	// targets and dependencies and run if any target is older
	// than any dependency, like make.
	LocalFileRelationship = "local-file"
)
