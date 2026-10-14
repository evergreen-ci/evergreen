package apimodels

import (
	"github.com/pkg/errors"
)

// CreatePatchModule holds the diff for a single module in a patch creation
// request. The diff is applied to the module's repository at the given githash.
type CreatePatchModule struct {
	ModuleName string `json:"module_name"`
	Githash    string `json:"githash"`
	Patch      string `json:"patch"`
}

// CreatePatchParameter is a parameter to set in the patch's project configuration.
type CreatePatchParameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CreatePatchRequest is the request body for creating a patch. The base repo
// diff ("patch") and module diffs are all optional: a patch with no diffs runs
// its tasks against the base commit. Repeat options are mutually exclusive with
// each other; each reuses the build variant/task definition from a previous
// patch instead of specifying variants and tasks.
type CreatePatchRequest struct {
	Description string `json:"description"`
	// BaseGithash is the commit the patch is based on. Required.
	BaseGithash string                 `json:"base_githash"`
	Patch       string                 `json:"patch"`
	Modules     []CreatePatchModule    `json:"modules" extensions:"x-nullable"`
	Parameters  []CreatePatchParameter `json:"parameters" extensions:"x-nullable"`

	Variants []string `json:"variants" extensions:"x-nullable"`
	Tasks    []string `json:"tasks" extensions:"x-nullable"`

	RegexVariants []string `json:"regex_variants" extensions:"x-nullable"`
	RegexTasks    []string `json:"regex_tasks" extensions:"x-nullable"`

	RegexTestSelectionVariants         []string `json:"regex_test_selection_variants" extensions:"x-nullable"`
	RegexTestSelectionExcludedVariants []string `json:"regex_test_selection_excluded_variants" extensions:"x-nullable"`
	RegexTestSelectionTasks            []string `json:"regex_test_selection_tasks" extensions:"x-nullable"`
	RegexTestSelectionExcludedTasks    []string `json:"regex_test_selection_excluded_tasks" extensions:"x-nullable"`

	Alias          string   `json:"alias"`
	Aliases        []string `json:"aliases" extensions:"x-nullable"`
	TriggerAliases []string `json:"trigger_aliases" extensions:"x-nullable"`

	// RepeatDefinition reuses the definition of the user's most recent patch in
	// this project.
	RepeatDefinition bool `json:"reuse_last_definition"`
	// RepeatFailed reuses the failed tasks of the user's most recent patch in
	// this project (or of RepeatPatchId, when set).
	RepeatFailed bool `json:"repeat_failed"`
	// RepeatPatchId reuses the definition of the given patch.
	RepeatPatchId string `json:"repeat_patch_id"`

	// FinalizeResponse activates the patch after creating it, starting its
	// tasks. When false, the patch is created unactivated and can be activated
	// later by setting "activated" on PATCH /patches/{patch_id}.
	FinalizeResponse bool `json:"finalize"`
}

// Validate checks that the request is internally consistent.
func (r *CreatePatchRequest) Validate() error {
	if r.BaseGithash == "" {
		return errors.New("must specify 'base_githash'")
	}

	repeatOptions := 0
	for _, set := range []bool{r.RepeatDefinition, r.RepeatFailed, r.RepeatPatchId != ""} {
		if set {
			repeatOptions++
		}
	}
	if repeatOptions > 1 {
		return errors.New("'reuse_last_definition', 'repeat_failed', and 'repeat_patch_id' are mutually exclusive")
	}

	seenModules := map[string]bool{}
	for _, m := range r.Modules {
		if m.ModuleName == "" {
			return errors.New("module_name must not be empty")
		}
		if seenModules[m.ModuleName] {
			return errors.Errorf("module '%s' is specified more than once", m.ModuleName)
		}
		seenModules[m.ModuleName] = true
	}

	for _, p := range r.Parameters {
		if p.Key == "" {
			return errors.New("parameter key must not be empty")
		}
	}

	// Mirrors the CLI: finalizing a patch needs a task/variant selection,
	// whether given directly, through an alias, or reused via a repeat option.
	if r.FinalizeResponse && !r.repeatOptionsSet() && !r.anyAliasSet() && !r.anyTaskOrVariantSet() {
		return errors.New("cannot finalize a patch with no tasks or variants; specify variants/tasks, an alias, or a repeat option")
	}

	return nil
}

// repeatOptionsSet returns whether any repeat option is set.
func (r *CreatePatchRequest) repeatOptionsSet() bool {
	return r.RepeatDefinition || r.RepeatFailed || r.RepeatPatchId != ""
}

// anyAliasSet returns whether any patch alias option is set.
func (r *CreatePatchRequest) anyAliasSet() bool {
	return r.Alias != "" || len(r.Aliases) > 0 || len(r.TriggerAliases) > 0
}

// anyTaskOrVariantSet returns whether any task/variant selection option is set.
func (r *CreatePatchRequest) anyTaskOrVariantSet() bool {
	return len(r.Variants)+len(r.RegexVariants)+len(r.Tasks)+len(r.RegexTasks) > 0
}
