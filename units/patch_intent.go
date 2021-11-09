package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v3"
)

const (
	patchIntentJobName = "patch-intent-processor"
)

func init() {
	registry.AddJobType(patchIntentJobName,
		func() amboy.Job { return makePatchIntentProcessor() })
}

type patchIntentProcessor struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment

	IntentID   string           `bson:"intent_id" json:"intent_id" yaml:"intent_id"`
	IntentType string           `bson:"intent_type" json:"intent_type" yaml:"intent_type"`
	PatchID    mgobson.ObjectId `bson:"patch_id,omitempty" json:"patch_id" yaml:"patch_id"`

	user   *user.DBUser
	intent patch.Intent

	gitHubError string
}

// NewPatchIntentProcessor creates an amboy job to create a patch from the
// given patch intent with the given object ID for the patch
func NewPatchIntentProcessor(patchID mgobson.ObjectId, intent patch.Intent) amboy.Job {
	j := makePatchIntentProcessor()
	j.IntentID = intent.ID()
	j.IntentType = intent.GetType()
	j.PatchID = patchID
	j.intent = intent

	j.SetID(fmt.Sprintf("%s-%s-%s", patchIntentJobName, j.IntentType, j.IntentID))
	return j
}

func makePatchIntentProcessor() *patchIntentProcessor {
	j := &patchIntentProcessor{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    patchIntentJobName,
				Version: 1,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *patchIntentProcessor) Run(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	githubOauthToken, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		j.AddError(err)
		return
	}

	if j.intent == nil {
		j.intent, err = patch.FindIntent(j.IntentID, j.IntentType)
		if err != nil {
			j.AddError(err)
			return
		}
		j.IntentType = j.intent.GetType()
	}

	patchDoc := j.intent.NewPatch()

	if err = j.finishPatch(ctx, patchDoc, githubOauthToken); err != nil {
		if j.IntentType == patch.GithubIntentType {
			if j.gitHubError == "" {
				j.gitHubError = OtherErrors
			}
			j.sendGitHubErrorStatus(patchDoc)
			grip.Error(message.WrapError(err, message.Fields{
				"job":          j.ID(),
				"message":      "sent github status error",
				"github_error": j.gitHubError,
				"owner":        patchDoc.GithubPatchData.BaseOwner,
				"repo":         patchDoc.GithubPatchData.BaseRepo,
				"pr_number":    patchDoc.GithubPatchData.PRNumber,
				"commit":       patchDoc.GithubPatchData.HeadHash,
				"project":      patchDoc.Project,
				"alias":        patchDoc.Alias,
				"patch_id":     patchDoc.Id.Hex(),
				"config_size":  len(patchDoc.PatchedConfig),
				"num_modules":  len(patchDoc.Patches),
			}))
		}
		j.AddError(err)
		return
	}

	if j.IntentType == patch.GithubIntentType {
		var update amboy.Job
		if len(patchDoc.Version) == 0 {
			update = NewGithubStatusUpdateJobForExternalPatch(patchDoc.Id.Hex())

		} else {
			update = NewGithubStatusUpdateJobForNewPatch(patchDoc.Id.Hex())
		}
		update.Run(ctx)
		j.AddError(update.Error())
		grip.Error(message.WrapError(update.Error(), message.Fields{
			"message":            "Failed to queue status update",
			"job":                j.ID(),
			"patch_id":           j.PatchID,
			"update_id":          update.ID(),
			"update_for_version": patchDoc.Version,
			"intent_type":        j.IntentType,
			"intent_id":          j.IntentID,
			"source":             "patch intents",
		}))

		j.AddError(model.AbortPatchesWithGithubPatchData(patchDoc.CreateTime,
			false, patchDoc.Id.Hex(), patchDoc.GithubPatchData.BaseOwner,
			patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.PRNumber))
	}
}

func (j *patchIntentProcessor) finishPatch(ctx context.Context, patchDoc *patch.Patch, githubOauthToken string) error {
	catcher := grip.NewBasicCatcher()

	var err error
	canFinalize := true
	switch j.IntentType {
	case patch.CliIntentType:
		catcher.Add(j.buildCliPatchDoc(ctx, patchDoc, githubOauthToken))

	case patch.GithubIntentType:
		canFinalize, err = j.buildGithubPatchDoc(ctx, patchDoc, githubOauthToken)
		if err != nil {
			if strings.Contains(err.Error(), thirdparty.Github502Error) {
				j.gitHubError = GitHubInternalError
			}
		}
		catcher.Add(err)

	case patch.TriggerIntentType:
		catcher.Add(j.buildTriggerPatchDoc(ctx, patchDoc))
	default:
		return errors.Errorf("Intent type '%s' is unknown", j.IntentType)
	}

	if err = catcher.Resolve(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":     "Failed to build patch document",
			"job":         j.ID(),
			"patch_id":    j.PatchID,
			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
			"source":      "patch intents",
		}))

		return err
	}

	if j.user == nil {
		j.user, err = user.FindOne(user.ById(patchDoc.Author))
		if err != nil {
			return err
		}
		if j.user == nil {
			return errors.New("Can't find patch author")
		}
	}

	if j.user.Settings.UseSpruceOptions.SpruceV1 {
		patchDoc.DisplayNewUI = true
	}

	pref, err := model.FindMergedProjectRef(patchDoc.Project, patchDoc.Version, true)
	if err != nil {
		return errors.Wrap(err, "can't find patch project")
	}
	if pref == nil {
		return errors.Errorf("no project ref '%s' found", patchDoc.Project)
	}

	// hidden projects can only run PR patches
	if !pref.IsEnabled() && (j.IntentType != patch.GithubIntentType || !pref.IsHidden()) {
		j.gitHubError = ProjectDisabled
		return errors.New("project is disabled")
	}

	if pref.IsPatchingDisabled() {
		j.gitHubError = PatchingDisabled
		return errors.New("patching is disabled for project")
	}

	if patchDoc.IsBackport() && !pref.CommitQueue.IsEnabled() {
		return errors.New("commit queue is disabled for project")
	}

	if !pref.TaskSync.IsPatchEnabled() && (len(patchDoc.SyncAtEndOpts.Tasks) != 0 || len(patchDoc.SyncAtEndOpts.BuildVariants) != 0) {
		j.gitHubError = PatchTaskSyncDisabled
		return errors.New("task sync at the end of a patched task is disabled by project settings")
	}

	validationCatcher := grip.NewBasicCatcher()
	// Get and validate patched config
	project, projectYaml, err := model.GetPatchedProject(ctx, patchDoc, githubOauthToken)
	if err != nil {
		if strings.Contains(err.Error(), model.EmptyConfigurationError) {
			j.gitHubError = EmptyConfig
		}
		if strings.Contains(err.Error(), thirdparty.Github502Error) {
			j.gitHubError = GitHubInternalError
		}
		if strings.Contains(err.Error(), model.LoadProjectError) {
			j.gitHubError = InvalidConfig
		}
		return errors.Wrap(err, "can't get patched config")
	}
	if errs := validator.CheckProjectSyntax(project, false); len(errs) != 0 {
		if errs = errs.AtLevel(validator.Error); len(errs) != 0 {
			validationCatcher.Errorf("invalid patched config syntax: %s", validator.ValidationErrorsToString(errs))
		}
	}
	if errs := validator.CheckProjectSettings(project, pref); len(errs) != 0 {
		if errs = errs.AtLevel(validator.Error); len(errs) != 0 {
			validationCatcher.Errorf("invalid patched config for current project settings: %s", validator.ValidationErrorsToString(errs))
		}
	}
	if validationCatcher.HasErrors() {
		j.gitHubError = ProjectFailsValidation
		return errors.Wrapf(validationCatcher.Resolve(), "patched project config has errors")
	}

	patchDoc.PatchedConfig = projectYaml

	for _, modulePatch := range patchDoc.Patches {
		if modulePatch.ModuleName != "" {
			// validate the module exists
			var module *model.Module
			module, err = project.GetModuleByName(modulePatch.ModuleName)
			if err != nil {
				return errors.Wrapf(err, "could not find module '%s'", modulePatch.ModuleName)
			}
			if module == nil {
				return errors.Errorf("no module named '%s'", modulePatch.ModuleName)
			}
		}
	}

	if j.intent.ReusePreviousPatchDefinition() {
		patchDoc.VariantsTasks, err = j.getPreviousPatchDefinition(project)
		if err != nil {
			return err
		}
	}

	// verify that all variants exists
	for _, buildVariant := range patchDoc.BuildVariants {
		if buildVariant == "all" || buildVariant == "" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			return errors.Errorf("No such buildvariant: '%s'", buildVariant)
		}
	}

	if len(patchDoc.VariantsTasks) == 0 {
		project.BuildProjectTVPairs(patchDoc, j.intent.GetAlias())
	}

	if (j.intent.ShouldFinalizePatch() || patchDoc.IsCommitQueuePatch()) &&
		len(patchDoc.VariantsTasks) == 0 {
		j.gitHubError = NoTasksOrVariants
		return errors.New("patch has no build variants or tasks")
	}

	if shouldTaskSync := len(patchDoc.SyncAtEndOpts.BuildVariants) != 0 || len(patchDoc.SyncAtEndOpts.Tasks) != 0; shouldTaskSync {
		patchDoc.SyncAtEndOpts.VariantsTasks = patchDoc.ResolveSyncVariantTasks(project.GetAllVariantTasks())
		// If the user requested task sync in their patch, it should match at least
		// one valid task in a build variant.
		if len(patchDoc.SyncAtEndOpts.VariantsTasks) == 0 {
			j.gitHubError = NoSyncTasksOrVariants
			return errors.Errorf("patch requests task sync for tasks '%s' in build variants '%s'"+
				" but did not match any tasks within any of the specified build variants",
				patchDoc.SyncAtEndOpts.Tasks, patchDoc.SyncAtEndOpts.BuildVariants)
		}
	}

	if patchDoc.IsCommitQueuePatch() {
		patchDoc.Description = model.MakeCommitQueueDescription(patchDoc.Patches, pref, project)
	}
	if patchDoc.IsBackport() {
		patchDoc.Description, err = patchDoc.MakeBackportDescription()
		if err != nil {
			return errors.Wrap(err, "can't make backport patch description")
		}
	}

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = j.user.IncPatchNumber()
	if err != nil {
		return errors.Wrap(err, "error computing patch num")
	}

	if patchDoc.CreateTime.IsZero() {
		patchDoc.CreateTime = time.Now()
	}
	patchDoc.Id = j.PatchID

	if _, err = ProcessTriggerAliases(ctx, patchDoc, pref, j.env, patchDoc.Triggers.Aliases); err != nil {
		return errors.Wrap(err, "problem processing trigger aliases")
	}

	if err = patchDoc.Insert(); err != nil {
		return err
	}

	if patchDoc.IsGithubPRPatch() {
		ghSub := event.NewGithubStatusAPISubscriber(event.GithubPullRequestSubscriber{
			Owner:    patchDoc.GithubPatchData.BaseOwner,
			Repo:     patchDoc.GithubPatchData.BaseRepo,
			PRNumber: patchDoc.GithubPatchData.PRNumber,
			Ref:      patchDoc.GithubPatchData.HeadHash,
		})
		patchSub := event.NewExpiringPatchOutcomeSubscription(j.PatchID.Hex(), ghSub)
		if err = patchSub.Upsert(); err != nil {
			catcher.Add(errors.Wrap(err, "failed to insert patch subscription for Github PR"))
		}
		buildSub := event.NewExpiringBuildOutcomeSubscriptionByVersion(j.PatchID.Hex(), ghSub)
		if err = buildSub.Upsert(); err != nil {
			catcher.Add(errors.Wrap(err, "failed to insert build subscription for Github PR"))
		}
		waitOnChilSub := event.NewGithubStatusAPISubscriber(event.GithubPullRequestSubscriber{
			Owner:    patchDoc.GithubPatchData.BaseOwner,
			Repo:     patchDoc.GithubPatchData.BaseRepo,
			PRNumber: patchDoc.GithubPatchData.PRNumber,
			Ref:      patchDoc.GithubPatchData.HeadHash,
			Type:     event.WaitOnChild,
		})
		if patchDoc.IsParent() {
			// add a subscription on each child patch to report it's status to github when it's done.
			for _, childPatch := range patchDoc.Triggers.ChildPatches {
				childGhStatusSub := event.NewGithubStatusAPISubscriber(event.GithubPullRequestSubscriber{
					Owner:    patchDoc.GithubPatchData.BaseOwner,
					Repo:     patchDoc.GithubPatchData.BaseRepo,
					PRNumber: patchDoc.GithubPatchData.PRNumber,
					Ref:      patchDoc.GithubPatchData.HeadHash,
					ChildId:  childPatch,
					Type:     event.SendChildPatchOutcome,
				})
				patchSub := event.NewExpiringPatchOutcomeSubscription(childPatch, childGhStatusSub)
				if err = patchSub.Upsert(); err != nil {
					catcher.Add(errors.Wrap(err, "failed to insert child patch subscription for Github PR"))
				}
				// add subscription so that the parent can wait on the children
				patchSub = event.NewExpiringPatchOutcomeSubscription(childPatch, waitOnChilSub)
				if err = patchSub.Upsert(); err != nil {
					catcher.Add(errors.Wrap(err, "failed to insert patch subscription for Github PR"))
				}

			}
		}
	}
	if patchDoc.IsBackport() {
		backportSubscription := event.NewExpiringPatchSuccessSubscription(j.PatchID.Hex(), event.NewEnqueuePatchSubscriber())
		if err = backportSubscription.Upsert(); err != nil {
			catcher.Add(errors.Wrap(err, "failed to insert backport subscription"))
		}
	}

	if catcher.HasErrors() {
		grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
			"message":     "failed to save subscription, patch will not notify",
			"job":         j.ID(),
			"patch_id":    j.PatchID,
			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
			"source":      "patch intents",
		}))
	}
	event.LogPatchStateChangeEvent(patchDoc.Id.Hex(), patchDoc.Status)

	if canFinalize && j.intent.ShouldFinalizePatch() {
		if _, err = model.FinalizePatch(ctx, patchDoc, j.intent.RequesterIdentity(), githubOauthToken); err != nil {
			if strings.Contains(err.Error(), thirdparty.Github502Error) {
				j.gitHubError = GitHubInternalError
			}
			grip.Error(message.WrapError(err, message.Fields{
				"message":     "Failed to finalize patch document",
				"job":         j.ID(),
				"patch_id":    j.PatchID,
				"intent_type": j.IntentType,
				"intent_id":   j.IntentID,
				"source":      "patch intents",
			}))
			return err
		}
		if j.IntentType == patch.CliIntentType {
			grip.Info(message.Fields{
				"operation":     "patch creation",
				"message":       "finalized patch at time of patch creation",
				"from":          "CLI",
				"job":           j.ID(),
				"patch_id":      patchDoc.Id,
				"variants":      patchDoc.BuildVariants,
				"tasks":         patchDoc.Tasks,
				"variant_tasks": patchDoc.VariantsTasks,
				"alias":         patchDoc.Alias,
			})
		}
	}

	return catcher.Resolve()
}

func (j *patchIntentProcessor) getPreviousPatchDefinition(project *model.Project) ([]patch.VariantTasks, error) {
	previousPatch, err := patch.FindOne(patch.MostRecentPatchByUserAndProject(j.user.Username(), project.Identifier))
	if err != nil {
		return nil, errors.Wrap(err, "error querying for most recent patch")
	}
	if previousPatch == nil {
		return nil, errors.Errorf("no previous patch available")
	}

	var res []patch.VariantTasks
	for _, vt := range previousPatch.VariantsTasks {
		tasksInProjectVariant := project.FindTasksForVariant(vt.Variant)
		displayTasksInProjectVariant := project.FindDisplayTasksForVariant(vt.Variant)

		// I want the subset of vt.tasks that exists in tasksForVariant
		tasks := utility.StringSliceIntersection(tasksInProjectVariant, vt.Tasks)
		var displayTasks []patch.DisplayTask
		for _, dt := range vt.DisplayTasks {
			if utility.StringSliceContains(displayTasksInProjectVariant, dt.Name) {
				displayTasks = append(displayTasks, patch.DisplayTask{Name: dt.Name})
			}
		}

		if len(tasks)+len(displayTasks) > 0 {
			res = append(res, patch.VariantTasks{
				Variant:      vt.Variant,
				Tasks:        tasks,
				DisplayTasks: displayTasks,
			})
		}
	}
	return res, nil
}

func ProcessTriggerAliases(ctx context.Context, p *patch.Patch, projectRef *model.ProjectRef, env evergreen.Environment, aliasNames []string) ([]string, error) {
	if len(aliasNames) == 0 {
		return nil, nil
	}

	type aliasGroup struct {
		project        string
		status         string
		parentAsModule string
	}
	aliasGroups := make(map[aliasGroup][]patch.PatchTriggerDefinition)
	for _, aliasName := range aliasNames {
		alias, found := projectRef.GetPatchTriggerAlias(aliasName)
		if !found {
			return nil, errors.Errorf("patch trigger alias '%s' is not defined", aliasName)
		}

		// group patches on project, status, parentAsModule
		group := aliasGroup{
			project:        alias.ChildProject,
			status:         alias.Status,
			parentAsModule: alias.ParentAsModule,
		}
		aliasGroups[group] = append(aliasGroups[group], alias)
	}

	triggerIntents := make([]patch.Intent, 0, len(aliasGroups))
	childPatchIds := make([]string, 0, len(aliasGroups))
	for group, definitions := range aliasGroups {
		triggerIntent := patch.NewTriggerIntent(patch.TriggerIntentOptions{
			ParentID:       p.Id.Hex(),
			ParentStatus:   group.status,
			ProjectID:      group.project,
			ParentAsModule: group.parentAsModule,
			Requester:      p.GetRequester(),
			Author:         p.Author,
			Definitions:    definitions,
		})

		if err := triggerIntent.Insert(); err != nil {
			return nil, errors.Wrap(err, "problem inserting trigger intent")
		}

		triggerIntents = append(triggerIntents, triggerIntent)
		childPatchIds = append(childPatchIds, triggerIntent.ID())
	}
	p.Triggers.ChildPatches = append(p.Triggers.ChildPatches, childPatchIds...)

	for _, intent := range triggerIntents {
		triggerIntent, ok := intent.(*patch.TriggerIntent)
		if !ok {
			return nil, errors.Errorf("intent '%s' didn't not have expected type '%T'", intent.ID(), intent)
		}

		job := NewPatchIntentProcessor(mgobson.ObjectIdHex(intent.ID()), intent)
		if triggerIntent.ParentStatus == "" {
			// In order to be able to finalize a patch from the CLI,
			// we need the child patch intents to exist when the parent patch is finalized.
			job.Run(ctx)
			if err := job.Error(); err != nil {
				return nil, errors.Wrap(err, "problem processing child patch")
			}
		} else {
			if err := env.RemoteQueue().Put(ctx, job); err != nil {
				return nil, errors.Wrap(err, "problem enqueueing child patch processing")
			}
		}
	}
	return childPatchIds, nil
}

func (j *patchIntentProcessor) buildCliPatchDoc(ctx context.Context, patchDoc *patch.Patch, githubOauthToken string) error {
	defer func() {
		grip.Error(message.WrapError(j.intent.SetProcessed(), message.Fields{
			"message":     "could not mark patch intent as processed",
			"intent_id":   j.IntentID,
			"intent_type": j.IntentType,
			"patch_id":    j.PatchID,
			"source":      "patch intents",
			"job":         j.ID(),
		}))
	}()

	projectRef, err := model.FindMergedProjectRef(patchDoc.Project, patchDoc.Version, true)
	if err != nil {
		return errors.Wrapf(err, "Could not find project ref '%s'", patchDoc.Project)
	}
	if projectRef == nil {
		return errors.Errorf("Could not find project ref '%s'", patchDoc.Project)
	}

	if patchDoc.IsBackport() {
		return j.buildBackportPatchDoc(ctx, projectRef, patchDoc)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	commit, err := thirdparty.GetCommitEvent(ctx, githubOauthToken, projectRef.Owner,
		projectRef.Repo, patchDoc.Githash)
	if err != nil {
		return errors.Wrapf(err, "could not find base revision '%s' for project '%s'",
			patchDoc.Githash, projectRef.Id)
	}
	// With `evergreen patch-file`, a user can pass a branch name or tag instead of a hash. We
	// must normalize this to a hash before storing the patch doc.
	if commit != nil && commit.SHA != nil && patchDoc.Githash != *commit.SHA {
		patchDoc.Githash = *commit.SHA
	}

	if len(patchDoc.Patches) > 0 {
		if patchDoc.Patches[0], err = getModulePatch(patchDoc.Patches[0]); err != nil {
			return errors.Wrap(err, "problem getting ModulePatch from GridFS")
		}
	}

	return nil
}

// getModulePatch reads the patch from GridFS, processes it, and
// stores the resulting summaries in the returned ModulePatch
func getModulePatch(modulePatch patch.ModulePatch) (patch.ModulePatch, error) {
	patchContents, err := patch.FetchPatchContents(modulePatch.PatchSet.PatchFileId)
	if err != nil {
		return modulePatch, errors.Wrap(err, "can't fetch patch contents")
	}

	var summaries []thirdparty.Summary
	if patch.IsMailboxDiff(patchContents) {
		var commitMessages []string
		summaries, commitMessages, err = thirdparty.GetPatchSummariesFromMboxPatch(patchContents)
		if err != nil {
			return modulePatch, errors.Wrapf(err, "error getting summaries by commit")
		}
		modulePatch.PatchSet.CommitMessages = commitMessages
	} else {
		summaries, err = thirdparty.GetPatchSummaries(patchContents)
		if err != nil {
			return modulePatch, errors.Wrap(err, "error getting patch summaries")
		}
	}

	modulePatch.IsMbox = len(patchContents) == 0 || patch.IsMailboxDiff(patchContents)
	modulePatch.ModuleName = ""
	modulePatch.PatchSet.Summary = summaries
	return modulePatch, nil
}

func (j *patchIntentProcessor) buildBackportPatchDoc(ctx context.Context, projectRef *model.ProjectRef, patchDoc *patch.Patch) error {
	if len(patchDoc.BackportOf.PatchID) > 0 {
		existingMergePatch, err := patch.FindOneId(patchDoc.BackportOf.PatchID)
		if err != nil {
			return errors.Wrap(err, "can't get existing merge patch")
		}
		if existingMergePatch == nil {
			return errors.Errorf("patch '%s' does not exist", patchDoc.BackportOf.PatchID)
		}
		if !existingMergePatch.IsCommitQueuePatch() {
			return errors.Errorf("can only backport commit queue patches")
		}

		for _, p := range existingMergePatch.Patches {
			if p.ModuleName == "" {
				p.Githash = patchDoc.Githash
			}
			patchDoc.Patches = append(patchDoc.Patches, p)
		}
		return nil
	}

	patchSet, err := patch.CreatePatchSetForSHA(ctx, j.env.Settings(), projectRef.Owner, projectRef.Repo, patchDoc.BackportOf.SHA)
	if err != nil {
		return errors.Wrapf(err, "can't create a patch set for SHA '%s'", patchDoc.BackportOf.SHA)
	}
	patchDoc.Patches = []patch.ModulePatch{{
		ModuleName: "",
		IsMbox:     true,
		PatchSet:   patchSet,
		Githash:    patchDoc.Githash,
	}}

	return nil
}

func (j *patchIntentProcessor) buildGithubPatchDoc(ctx context.Context, patchDoc *patch.Patch, githubOauthToken string) (bool, error) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return false, errors.Wrap(err, "github pr testing is disabled, error retrieving admin settings")
	}
	if flags.GithubPRTestingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     patchIntentJobName,
			"message": "github pr testing is disabled, not processing pull request",

			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
		})
		return false, errors.New("github pr testing is disabled, not processing pull request")
	}
	defer func() {
		grip.Error(message.WrapError(j.intent.SetProcessed(), message.Fields{
			"message":     "could not mark patch intent as processed",
			"intent_id":   j.IntentID,
			"intent_type": j.IntentType,
			"patch_id":    j.PatchID,
			"source":      "patch intents",
			"job":         j.ID(),
		}))
	}()

	mustBeMemberOfOrg := j.env.Settings().GithubPRCreatorOrg
	if mustBeMemberOfOrg == "" {
		return false, errors.New("Github PR testing not configured correctly; requires a Github org to authenticate against")
	}

	projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch)
	if err != nil {
		return false, errors.Wrapf(err, "Could not fetch project ref for repo '%s/%s' with branch '%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.BaseBranch)
	}
	if projectRef == nil {
		return false, errors.Errorf("Could not find project ref for repo '%s/%s' with branch '%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.BaseBranch)
	}

	if len(projectRef.GithubTriggerAliases) > 0 {
		patchDoc.Triggers = patch.TriggerInfo{Aliases: projectRef.GithubTriggerAliases}
	}

	isMember, err := j.authAndFetchPRMergeBase(ctx, patchDoc, mustBeMemberOfOrg,
		patchDoc.GithubPatchData.Author, githubOauthToken)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":     "github API failure",
			"source":      "patch intents",
			"job":         j.ID(),
			"patch_id":    j.PatchID,
			"base_repo":   fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":   fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":   patchDoc.GithubPatchData.PRNumber,
			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
		}))
		return false, err
	}

	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(ctx, githubOauthToken, patchDoc.GithubPatchData)
	if err != nil {
		return isMember, err
	}

	patchFileID := fmt.Sprintf("%s_%s", patchDoc.Id.Hex(), patchDoc.Githash)
	patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
		ModuleName: "",
		Githash:    patchDoc.Githash,
		PatchSet: patch.PatchSet{
			PatchFileId: patchFileID,
			Summary:     summaries,
		},
	})
	patchDoc.Project = projectRef.Id

	if err = db.WriteGridFile(patch.GridFSPrefix, patchFileID, strings.NewReader(patchContent)); err != nil {
		return isMember, errors.Wrap(err, "failed to write patch file to db")
	}

	j.user, err = findEvergreenUserForPR(patchDoc.GithubPatchData.AuthorUID)
	if err != nil {
		return isMember, errors.Wrap(err, "failed to fetch user")
	}
	patchDoc.Author = j.user.Id

	return isMember, nil
}

func (j *patchIntentProcessor) buildTriggerPatchDoc(ctx context.Context, patchDoc *patch.Patch) error {
	defer func() {
		grip.Error(message.WrapError(j.intent.SetProcessed(), message.Fields{
			"message":     "could not mark patch intent as processed",
			"intent_id":   j.IntentID,
			"intent_type": j.IntentType,
			"patch_id":    j.PatchID,
			"source":      "patch intents",
			"job":         j.ID(),
		}))
	}()

	intent, ok := j.intent.(*patch.TriggerIntent)
	if !ok {
		return errors.Errorf("intent '%s' didn't not have expected type '%T'", j.IntentID, j.intent)
	}

	v, project, err := model.FindLatestVersionWithValidProject(patchDoc.Project)
	if err != nil {
		return errors.Wrapf(err, "problem getting last known project for '%s'", patchDoc.Project)
	}

	matchingTasks, err := project.VariantTasksForSelectors(intent.Definitions, patchDoc.GetRequester())
	if err != nil {
		return errors.Wrap(err, "problem matching tasks to alias definitions")
	}
	if len(matchingTasks) == 0 {
		return nil
	}

	yamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return errors.Wrap(err, "can't marshal child project")
	}

	patchDoc.Githash = v.Revision
	patchDoc.PatchedConfig = string(yamlBytes)
	patchDoc.VariantsTasks = matchingTasks

	if intent.ParentAsModule != "" {
		parentPatch, err := patch.FindOneId(patchDoc.Triggers.ParentPatch)
		if err != nil {
			return errors.Wrap(err, "can't get parent patch")
		}
		if parentPatch == nil {
			return errors.Errorf("parent patch '%s' does not exist", patchDoc.Triggers.ParentPatch)
		}
		for _, p := range parentPatch.Patches {
			if p.ModuleName == "" {
				patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
					ModuleName: intent.ParentAsModule,
					PatchSet:   p.PatchSet,
				})
				break
			}
		}
	}
	return nil
}

func findEvergreenUserForPR(githubUID int) (*user.DBUser, error) {
	// try and find a user by github uid
	u, err := user.FindByGithubUID(githubUID)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return u, nil
	}

	// Otherwise, use the github patch user
	u, err = user.FindOne(user.ById(evergreen.GithubPatchUser))
	if err != nil {
		return u, err
	}
	// and if that user doesn't exist, make it
	if u == nil {
		u = &user.DBUser{
			Id:       evergreen.GithubPatchUser,
			DispName: "Github Pull Requests",
			APIKey:   utility.RandomString(),
		}
		if err = u.Insert(); err != nil {
			return nil, errors.Wrap(err, "failed to create github pull request user")
		}
	}

	return u, err
}

func (j *patchIntentProcessor) authAndFetchPRMergeBase(ctx context.Context, patchDoc *patch.Patch, requiredOrganization, githubUser, githubOauthToken string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	isMember, err := thirdparty.GithubUserInOrganization(ctx, githubOauthToken, requiredOrganization, githubUser)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job":          j.ID(),
			"message":      "Failed to authenticate github PR",
			"source":       "patch intents",
			"creator":      githubUser,
			"required_org": requiredOrganization,
			"base_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":    patchDoc.GithubPatchData.PRNumber,
		}))
		return false, err

	}

	hash, err := thirdparty.GetPullRequestMergeBase(ctx, githubOauthToken, patchDoc.GithubPatchData)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job":          j.ID(),
			"message":      "Failed to authenticate github PR",
			"source":       "patch intents",
			"creator":      githubUser,
			"required_org": requiredOrganization,
			"base_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":    patchDoc.GithubPatchData.PRNumber,
		}))
		return isMember, err
	}

	patchDoc.Githash = hash

	return isMember, nil
}

func (j *patchIntentProcessor) sendGitHubErrorStatus(patchDoc *patch.Patch) {
	update := NewGithubStatusUpdateJobForProcessingError(
		evergreenContext,
		patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo,
		patchDoc.GithubPatchData.HeadHash,
		j.gitHubError,
	)
	update.Run(nil)

	j.AddError(update.Error())
}
