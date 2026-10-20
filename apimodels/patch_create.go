package apimodels

import (
	"github.com/pkg/errors"
)

// CreatePatchModule holds the diff for a single module in a patch creation
// request. The diff is applied to the module's repository at the given githash.
type CreatePatchModule struct {
	// ModuleName is the name of the module, as defined in the project
	// configuration, that the diff applies to. Required.
	ModuleName string `json:"module_name"`
	// Githash is the commit the module diff is based on. Required.
	Githash string `json:"githash"`
	// Patch is the unified diff to apply to the module. Required.
	Patch string `json:"patch"`
}

// CreatePatchParameter is a parameter to set in the patch's project configuration.
type CreatePatchParameter struct {
	// Key is the name of the parameter, which must match a parameter defined
	// in the project configuration. Required.
	Key string `json:"key"`
	// Value is the value to set the parameter to. Required.
	Value string `json:"value"`
}

// CreatePatchRequest is the request body for creating a patch. The base repo
// diff ("patch") and module diffs are all optional: a patch with no diffs runs
// its tasks against the base commit. Repeat options are mutually exclusive with
// each other; each reuses the build variant/task definition from a previous
// patch instead of specifying variants and tasks.
type CreatePatchRequest struct {
	// Description is a human-readable summary of the patch, shown in the UI.
	Description string `json:"description"`
	// BaseGithash is the commit the patch is based on, in the project's
	// repository. Required.
	BaseGithash string `json:"base_githash"`
	// Patch is the unified diff to apply to the base commit in the project's
	// repository. Optional: a patch with no diffs runs its tasks against the
	// base commit.
	Patch string `json:"patch"`
	// Modules holds diffs for the project's modules, applied alongside the
	// base diff. Optional.
	Modules []CreatePatchModule `json:"modules" extensions:"x-nullable"`
	// Parameters sets values for parameters defined in the project
	// configuration. Optional.
	Parameters []CreatePatchParameter `json:"parameters" extensions:"x-nullable"`

	// Variants is the list of build variants to run. Optional, unless
	// finalizing.
	Variants []string `json:"variants" extensions:"x-nullable"`
	// Tasks is the list of tasks to run in the selected variants. Optional,
	// unless finalizing.
	Tasks []string `json:"tasks" extensions:"x-nullable"`

	// RegexVariants selects build variants to run by regular expression, as an
	// alternative to Variants. Optional, unless finalizing.
	RegexVariants []string `json:"regex_variants" extensions:"x-nullable"`
	// RegexTasks selects tasks to run by regular expression, as an
	// alternative to Tasks. Optional, unless finalizing.
	RegexTasks []string `json:"regex_tasks" extensions:"x-nullable"`

	// RegexTestSelectionVariants selects build variants for test selection by
	// regular expression. Optional.
	RegexTestSelectionVariants []string `json:"regex_test_selection_variants" extensions:"x-nullable"`
	// RegexTestSelectionExcludedVariants excludes build variants from test
	// selection by regular expression. Optional.
	RegexTestSelectionExcludedVariants []string `json:"regex_test_selection_excluded_variants" extensions:"x-nullable"`
	// RegexTestSelectionTasks selects tasks for test selection by regular
	// expression. Optional.
	RegexTestSelectionTasks []string `json:"regex_test_selection_tasks" extensions:"x-nullable"`
	// RegexTestSelectionExcludedTasks excludes tasks from test selection by
	// regular expression. Optional.
	RegexTestSelectionExcludedTasks []string `json:"regex_test_selection_excluded_tasks" extensions:"x-nullable"`

	// Alias is a single patch alias that resolves the build variants and tasks
	// to run. Optional.
	Alias string `json:"alias"`
	// Aliases is a list of patch aliases that resolve the build variants and
	// tasks to run, combined. Optional.
	Aliases []string `json:"aliases" extensions:"x-nullable"`
	// TriggerAliases is a list of the project's trigger aliases to run.
	// Optional.
	TriggerAliases []string `json:"trigger_aliases" extensions:"x-nullable"`

	// RepeatLastPatch reuses the build variant/task definition of the user's
	// most recent patch in this project. Mutually exclusive with RepeatFailed
	// and RepeatPatchId.
	RepeatLastPatch bool `json:"repeat_last_patch"`
	// RepeatFailed reuses the failed tasks of the user's most recent patch in
	// this project (or of RepeatPatchId, when set). Mutually exclusive with
	// RepeatLastPatch and RepeatPatchId.
	RepeatFailed bool `json:"repeat_failed"`
	// RepeatPatchId reuses the build variant/task definition of the given
	// patch. Mutually exclusive with RepeatLastPatch and RepeatFailed.
	RepeatPatchId string `json:"repeat_patch_id"`

	// Finalize activates the patch after creating it, starting its tasks. When
	// false, the patch is created unactivated and can be activated later by
	// setting "activated" on PATCH /patches/{patch_id}. A finalized patch
	// needs both a variant selection and a task selection, whether given
	// directly, through an alias, or reused via a repeat option.
	Finalize bool `json:"finalize"`
}

// Validate checks that the request is internally consistent.
func (r *CreatePatchRequest) Validate() error {
	if r.BaseGithash == "" {
		return errors.New("must specify 'base_githash'")
	}

	repeatOptions := 0
	for _, set := range []bool{r.RepeatLastPatch, r.RepeatFailed, r.RepeatPatchId != ""} {
		if set {
			repeatOptions++
		}
	}
	if repeatOptions > 1 {
		return errors.New("'repeat_last_patch', 'repeat_failed', and 'repeat_patch_id' are mutually exclusive")
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

	// Mirrors the CLI: finalizing a patch needs both a variant selection and a
	// task selection, whether given directly, through an alias, or reused via
	// a repeat option.
	if r.Finalize && !r.repeatOptionsSet() && !r.anyAliasSet() {
		if len(r.Variants)+len(r.RegexVariants) == 0 {
			return errors.New("cannot finalize a patch with no variants; specify variants, an alias, or a repeat option")
		}
		if len(r.Tasks)+len(r.RegexTasks) == 0 {
			return errors.New("cannot finalize a patch with no tasks; specify tasks, an alias, or a repeat option")
		}
	}

	return nil
}

// repeatOptionsSet returns whether any repeat option is set.
func (r *CreatePatchRequest) repeatOptionsSet() bool {
	return r.RepeatLastPatch || r.RepeatFailed || r.RepeatPatchId != ""
}

// anyAliasSet returns whether any patch alias option is set.
func (r *CreatePatchRequest) anyAliasSet() bool {
	return r.Alias != "" || len(r.Aliases) > 0 || len(r.TriggerAliases) > 0
}
