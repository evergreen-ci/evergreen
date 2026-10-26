package manifest

import "sort"

const Collection = "manifest"

// ModuleSelectionRule describes how a manifest module's revision was chosen.
type ModuleSelectionRule string

const (
	// ModuleSelectionPinnedRef is the module's ref from the YAML.
	ModuleSelectionPinnedRef ModuleSelectionRule = "pinned_ref"
	// ModuleSelectionTriggerCommit is the module commit that fired the push
	// trigger that created the version.
	ModuleSelectionTriggerCommit ModuleSelectionRule = "trigger_commit"
	// ModuleSelectionLatestIngested is the newest module commit Evergreen
	// ingested as of the version's ingest time.
	ModuleSelectionLatestIngested ModuleSelectionRule = "latest_ingested"
	// ModuleSelectionGitHubFallback is the newest commit on the module branch
	// according to GitHub, used when Evergreen hasn't ingested any.
	ModuleSelectionGitHubFallback ModuleSelectionRule = "github_fallback"
	// ModuleSelectionBaseVersion is inherited from the base version's manifest.
	ModuleSelectionBaseVersion ModuleSelectionRule = "base_version"
	// ModuleSelectionCarriedForward is kept from the most recent earlier
	// mainline or trigger version's manifest, either because a different
	// module triggered the version or because the latest commit Evergreen
	// knows about is older.
	ModuleSelectionCarriedForward ModuleSelectionRule = "carried_forward"
)

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
	Modules map[string]*Module `json:"modules" bson:"modules" extensions:"x-nullable"`
	// True if the version is a mainline build.
	IsBase bool `json:"is_base" bson:"is_base"`
	// PreviousVersionID is the version ChangedModules is relative to. It's the
	// base version for patches, and the version before this one on the
	// waterfall otherwise. Empty if that version had no manifest to compare
	// against, in which case ChangedModules is also empty.
	PreviousVersionID string `json:"previous_version_id,omitempty" bson:"previous_version_id,omitempty"`
	// ChangedModules are the modules added, removed, or moved to a different
	// commit since PreviousVersionID.
	ChangedModules  []string          `json:"changed_modules,omitempty" bson:"changed_modules,omitempty"`
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
	// The rule that chose the revision. Empty for wiki modules and older manifests.
	SelectionRule ModuleSelectionRule `json:"selection_rule,omitempty" bson:"selection_rule,omitempty"`
}

// GetChangedModules returns the sorted names of the modules that were added,
// removed, or point to a different commit compared to prev.
func (m *Manifest) GetChangedModules(prev *Manifest) []string {
	var changed []string
	for name, mod := range m.Modules {
		prevMod, ok := prev.Modules[name]
		if !ok || !mod.sameCommit(prevMod) {
			changed = append(changed, name)
		}
	}
	for name := range prev.Modules {
		if _, ok := m.Modules[name]; !ok {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

func (m *Module) sameCommit(other *Module) bool {
	if m == nil || other == nil {
		return m == other
	}
	return m.Owner == other.Owner && m.Repo == other.Repo && m.Branch == other.Branch && m.Revision == other.Revision
}
