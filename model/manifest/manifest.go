package manifest

const Collection = "manifest"

// Manifest is a representation of the modules associated with a version.
// Id is the version id,
// Revision is the revision of the version on the project
// ProjectName is the Project Id,
// Branch is the branch of the repository. Modules is a map of the GitHub repository name to the
// Module's information associated with the specific version.
type Manifest struct {
	// Identifier for the version.
	Id string `json:"id" bson:"_id"`
	// The revision of the version.
	Revision string `json:"revision" bson:"revision"`
	// The project identifier for the version.
	ProjectName string `json:"project" bson:"project"`
	// The branch of the repository.
	Branch string `json:"branch" bson:"branch"`
	// Map from the GitHub repository name to the module's information.
	Modules map[string]*Module `json:"modules" bson:"modules"`
	// True if the version is a mainline build.
	IsBase          bool              `json:"is_base" bson:"is_base"`
	ModuleOverrides map[string]string `json:"module_overrides,omitempty" bson:"-"`
}

// A Module is a snapshot of the module associated with a version.
// Branch is the branch of the repository,
// Repo is the name of the repository,
// Revision is the revision of the head of the branch,
// Owner is the owner of the repository,
// URL is the url to the GitHub API call to that specific commit.
type Module struct {
	// The branch of the repository.
	Branch string `json:"branch" bson:"branch"`
	// The name of the repository.
	Repo string `json:"repo" bson:"repo"`
	// The revision of the head of the branch.
	Revision string `json:"revision" bson:"revision"`
	// The owner of the repository.
	Owner string `json:"owner" bson:"owner"`
	// The url to the GitHub API call to that specific commit.
	URL string `json:"url" bson:"url"`
}
