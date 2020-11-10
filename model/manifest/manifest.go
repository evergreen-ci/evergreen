package manifest

const Collection = "manifest"

// Manifest is a representation of the modules associated with the a version.
// Id is the version id,
// Revision is the revision of the version on the project
// ProjectName is the Project Id,
// Branch is the branch of the repository. Modules is a map of the GitHub repository name to the
// Module's information associated with the specific version.
type Manifest struct {
	Id              string             `json:"id" bson:"_id"`
	Revision        string             `json:"revision" bson:"revision"`
	ProjectName     string             `json:"project" bson:"project"`
	Branch          string             `json:"branch" bson:"branch"`
	Modules         map[string]*Module `json:"modules" bson:"modules"`
	IsBase          bool               `json:"is_base" bson:"is_base"`
	ModuleOverrides map[string]string  `json:"module_overrides,omitempty" bson:"-"`
}

// A Module is a snapshot of the module associated with a version.
// Branch is the branch of the repository,
// Repo is the name of the repository,
// Revision is the revision of the head of the branch,
// Owner is the owner of the repository,
// URL is the url to the GitHub API call to that specific commit.
type Module struct {
	Branch   string `json:"branch" bson:"branch"`
	Repo     string `json:"repo" bson:"repo"`
	Revision string `json:"revision" bson:"revision"`
	Owner    string `json:"owner" bson:"owner"`
	URL      string `json:"url" bson:"url"`
}
