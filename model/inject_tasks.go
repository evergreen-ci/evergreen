package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// InjectTasksIntoVersion adds the given task/variant pairs to an already-created
// version in place (reusing the generate.tasks machinery) and returns the IDs of
// the tasks that were newly created. Pairs whose task already exists in the
// version are skipped, so repeated calls with the same pairs are idempotent.
// The tasks must already be defined in the project config; the parser project is
// not modified. The patch's VariantsTasks is updated so the injected tasks
// survive a later reconfigure or restart.
func InjectTasksIntoVersion(ctx context.Context, settings *evergreen.Settings, v *Version, p *Project, pairs TaskVariantPairs) ([]string, error) {
	existingBuilds, err := build.Find(ctx, build.ByVersion(v.Id))
	if err != nil {
		return nil, errors.Wrapf(err, "finding builds for version '%s'", v.Id)
	}
	buildSet := map[string]struct{}{}
	for _, b := range existingBuilds {
		buildSet[b.BuildVariant] = struct{}{}
	}

	// Compute the delta of pairs that don't yet exist in the version so that
	// repeated calls with the same pairs are idempotent.
	delta, err := deltaPairsNotInVersion(ctx, v, pairs)
	if err != nil {
		return nil, errors.Wrapf(err, "computing task/variant pairs to inject into version '%s'", v.Id)
	}
	if len(delta.ExecTasks) == 0 && len(delta.DisplayTasks) == 0 {
		return nil, nil
	}

	projectRef, err := FindMergedProjectRef(ctx, p.Identifier, v.Id, true)
	if err != nil {
		return nil, errors.Wrapf(err, "finding merged project ref '%s' for version '%s'", p.Identifier, v.Id)
	}
	if projectRef == nil {
		return nil, errors.Errorf("project '%s' not found", p.Identifier)
	}

	if v.Requester == evergreen.GithubPRRequester {
		numCheckRuns := p.GetNumCheckRunsFromTaskVariantPairs(&delta)
		if err := VerifyCheckRunLimit(numCheckRuns, settings.GitHubCheckRun.CheckRunLimit, projectRef.HasGitHubAppAuth(ctx)); err != nil {
			return nil, err
		}
	}

	pairsForExistingVariants, pairsForNewVariants := splitPairsByExistingVariants(delta, buildSet)

	taskIDs, err := NewTaskIdConfig(p, v, delta, projectRef.Identifier)
	if err != nil {
		return nil, errors.Wrap(err, "creating task ID table for task/variant pairs to inject")
	}
	if err := validateGeneratedProjectMaxTasks(ctx, v, "", taskIDs.Length()); err != nil {
		return nil, errors.Wrapf(err, "validating the number of tasks to inject into version '%s'", v.Id)
	}

	// The task ID config was built solely from the delta, so every entry it
	// contains corresponds to a task that will be newly created. Capture these
	// IDs now because the task-creation calls below mutate the config's maps to
	// include IDs of pre-existing tasks.
	createdTaskIDs := make([]string, 0, taskIDs.Length())
	for _, id := range taskIDs.ExecutionTasks {
		createdTaskIDs = append(createdTaskIDs, id)
	}
	for _, id := range taskIDs.DisplayTasks {
		createdTaskIDs = append(createdTaskIDs, id)
	}

	creationInfo := TaskCreationInfo{
		Project:    p,
		ProjectRef: projectRef,
		Version:    v,
		TaskIDs:    taskIDs,
		Pairs:      pairsForExistingVariants,
		// Injected tasks must follow the version's activation state so that an
		// unactivated version (e.g. an outside-org PR pending authorization) is
		// neither itself activated nor given an activated build/task from an
		// injected new variant.
		RespectVersionActivation: true,
	}
	// Only populated for patches, not mainline commits.
	if evergreen.IsPatchRequester(v.Requester) {
		patchDoc, err := patch.FindOneId(ctx, v.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "finding patch '%s'", v.Id)
		}
		if patchDoc != nil {
			tsParams, err := newTestSelectionParams(patchDoc)
			if err != nil {
				return nil, errors.Wrap(err, "making test selection params for task creation")
			}
			creationInfo.TestSelectionParams = *tsParams
		}
	}

	if _, _, err := addNewTasksToExistingBuilds(ctx, creationInfo, existingBuilds, evergreen.GenerateTasksActivator); err != nil {
		return nil, errors.Wrap(err, "adding injected tasks to existing builds")
	}

	creationInfo.Pairs = pairsForNewVariants
	if _, _, err := addNewBuilds(ctx, creationInfo, existingBuilds); err != nil {
		return nil, errors.Wrap(err, "adding new builds for injected tasks")
	}

	if err := updateBuildStatusesForGeneratedTasks(ctx, v.Id, pairsForExistingVariants); err != nil {
		return nil, errors.Wrap(err, "updating statuses for builds that have injected tasks")
	}

	if err := persistInjectedTasksToPatch(ctx, v.Id, delta); err != nil {
		return nil, errors.Wrapf(err, "persisting injected tasks to patch '%s'", v.Id)
	}

	return createdTaskIDs, nil
}

// deltaPairsNotInVersion returns the subset of pairs whose task does not already
// exist in the version.
func deltaPairsNotInVersion(ctx context.Context, v *Version, pairs TaskVariantPairs) (TaskVariantPairs, error) {
	existingTasks, err := task.FindAll(ctx, db.Query(task.ByVersion(v.Id)).WithFields(task.BuildVariantKey, task.DisplayNameKey))
	if err != nil {
		return TaskVariantPairs{}, errors.Wrapf(err, "finding tasks for version '%s'", v.Id)
	}
	existingPairs := map[TVPair]struct{}{}
	for _, t := range existingTasks {
		existingPairs[TVPair{Variant: t.BuildVariant, TaskName: t.DisplayName}] = struct{}{}
	}

	delta := TaskVariantPairs{}
	for _, pair := range pairs.ExecTasks {
		if _, ok := existingPairs[pair]; !ok {
			delta.ExecTasks = append(delta.ExecTasks, pair)
		}
	}
	for _, pair := range pairs.DisplayTasks {
		if _, ok := existingPairs[pair]; !ok {
			delta.DisplayTasks = append(delta.DisplayTasks, pair)
		}
	}
	return delta, nil
}

// splitPairsByExistingVariants partitions pairs into those whose variant already
// has a build in the version and those whose variant does not.
func splitPairsByExistingVariants(pairs TaskVariantPairs, buildSet map[string]struct{}) (existing, new TaskVariantPairs) {
	for _, execTask := range pairs.ExecTasks {
		if _, ok := buildSet[execTask.Variant]; ok {
			existing.ExecTasks = append(existing.ExecTasks, execTask)
		} else {
			new.ExecTasks = append(new.ExecTasks, execTask)
		}
	}
	for _, dispTask := range pairs.DisplayTasks {
		if _, ok := buildSet[dispTask.Variant]; ok {
			existing.DisplayTasks = append(existing.DisplayTasks, dispTask)
		} else {
			new.DisplayTasks = append(new.DisplayTasks, dispTask)
		}
	}
	return existing, new
}

// persistInjectedTasksToPatch unions the injected pairs into the patch's stored
// VariantsTasks so the injected tasks survive a later reconfigure or restart. It
// is a no-op if there is no patch for the version (e.g. mainline commits).
func persistInjectedTasksToPatch(ctx context.Context, versionID string, delta TaskVariantPairs) error {
	patchDoc, err := patch.FindOneId(ctx, versionID)
	if err != nil {
		return errors.Wrapf(err, "finding patch '%s'", versionID)
	}
	if patchDoc == nil {
		return nil
	}

	merged := VariantTasksToTVPairs(patchDoc.VariantsTasks)
	seenExec := map[TVPair]struct{}{}
	for _, pair := range merged.ExecTasks {
		seenExec[pair] = struct{}{}
	}
	seenDisplay := map[TVPair]struct{}{}
	for _, pair := range merged.DisplayTasks {
		seenDisplay[pair] = struct{}{}
	}
	for _, pair := range delta.ExecTasks {
		if _, ok := seenExec[pair]; !ok {
			merged.ExecTasks = append(merged.ExecTasks, pair)
		}
	}
	for _, pair := range delta.DisplayTasks {
		if _, ok := seenDisplay[pair]; !ok {
			merged.DisplayTasks = append(merged.DisplayTasks, pair)
		}
	}

	return errors.Wrap(patchDoc.SetVariantsTasks(ctx, merged.TVPairsToVariantTasks()), "setting patch variants and tasks")
}
