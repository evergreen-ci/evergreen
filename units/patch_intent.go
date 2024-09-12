package units

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	patchIntentJobName    = "patch-intent-processor"
	githubDependabotUser  = "dependabot[bot]"
	maxPatchIntentJobTime = 10 * time.Minute
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
// given patch intent. The patch ID is the new ID for the patch to be created,
// not the patch intent.
func NewPatchIntentProcessor(env evergreen.Environment, patchID mgobson.ObjectId, intent patch.Intent) amboy.Job {
	j := makePatchIntentProcessor()
	j.IntentID = intent.ID()
	j.IntentType = intent.GetType()
	j.PatchID = patchID
	j.intent = intent
	j.env = env

	j.SetID(fmt.Sprintf("%s-%s-%s", patchIntentJobName, j.IntentType, j.IntentID))
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxPatchIntentJobTime,
	})
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

	var err error
	if j.intent == nil {
		j.intent, err = patch.FindIntent(j.IntentID, j.IntentType)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding patch intent '%s'", j.IntentID))
			return
		}
		j.IntentType = j.intent.GetType()
	}

	patchDoc := j.intent.NewPatch()

	// set owner and repo for child patches
	if j.IntentType == patch.TriggerIntentType || j.IntentType == patch.CliIntentType {
		if patchDoc.Project == "" {
			j.AddError(errors.New("cannot search for an empty project"))
			return
		}
		p, err := model.FindBranchProjectRef(patchDoc.Project)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding project '%s'", patchDoc.Project))
			return
		}
		if p == nil {
			j.AddError(errors.Errorf("child project '%s' not found", patchDoc.Project))
			return
		}
		if j.IntentType == patch.TriggerIntentType {
			parentProject, err := model.FindBranchProjectRef(patchDoc.Triggers.ParentProjectID)
			if err != nil {
				j.AddError(errors.Wrapf(err, "finding project '%s'", patchDoc.Project))
				return
			}
			if parentProject == nil {
				j.AddError(errors.Errorf("parent project '%s' not found", patchDoc.Triggers.ParentPatch))
				return
			}
			if p.Owner == parentProject.Owner && p.Repo == parentProject.Repo &&
				p.Branch == parentProject.Branch {
				patchDoc.Triggers.SameBranchAsParent = true
			}
		}
		patchDoc.GithubPatchData.BaseOwner = p.Owner
		patchDoc.GithubPatchData.BaseRepo = p.Repo
	}

	if err = j.finishPatch(ctx, patchDoc); err != nil {
		if j.IntentType == patch.GithubIntentType || j.IntentType == patch.GithubMergeIntentType {
			if j.gitHubError == "" {
				j.gitHubError = OtherErrors
			}
			j.sendGitHubErrorStatus(ctx, patchDoc)
			msg := message.Fields{
				"job":          j.ID(),
				"message":      "sent GitHub status error",
				"github_error": j.gitHubError,
				"intent_type":  j.IntentType,
			}
			if j.IntentType == patch.GithubIntentType {
				msg["owner"] = patchDoc.GithubPatchData.BaseOwner
				msg["repo"] = patchDoc.GithubPatchData.BaseRepo
				msg["pr_number"] = patchDoc.GithubPatchData.PRNumber
				msg["commit"] = patchDoc.GithubPatchData.HeadHash
			} else if j.IntentType == patch.GithubMergeIntentType {
				msg["owner"] = patchDoc.GithubMergeData.Org
				msg["repo"] = patchDoc.GithubMergeData.Repo
				msg["base_branch"] = patchDoc.GithubMergeData.BaseBranch
				msg["head_branch"] = patchDoc.GithubMergeData.HeadBranch
				msg["head_sha"] = patchDoc.GithubMergeData.HeadSHA
			}
			grip.Error(message.WrapError(err, msg))
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
			"message":            "failed to queue status update",
			"job":                j.ID(),
			"patch_id":           j.PatchID,
			"update_id":          update.ID(),
			"update_for_version": patchDoc.Version,
			"intent_type":        j.IntentType,
			"intent_id":          j.IntentID,
			"source":             "patch intents",
		}))

		j.AddError(model.AbortPatchesWithGithubPatchData(ctx, patchDoc.CreateTime,
			false, patchDoc.Id.Hex(), patchDoc.GithubPatchData.BaseOwner,
			patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.PRNumber))
	}
}

func (j *patchIntentProcessor) finishPatch(ctx context.Context, patchDoc *patch.Patch) error {
	token, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "getting GitHub OAuth token")
	}
	catcher := grip.NewBasicCatcher()

	canFinalize := true
	var patchedProject *model.Project
	var patchedParserProject *model.ParserProject
	switch j.IntentType {
	case patch.CliIntentType:
		catcher.Wrap(j.buildCliPatchDoc(ctx, patchDoc, token), "building CLI patch document")
	case patch.GithubIntentType:
		canFinalize, err = j.buildGithubPatchDoc(ctx, patchDoc, token)
		if err != nil {
			if strings.Contains(err.Error(), thirdparty.Github502Error) {
				j.gitHubError = GitHubInternalError
			}
		}
		catcher.Wrap(err, "building GitHub patch document")
	case patch.GithubMergeIntentType:
		if err := j.buildGithubMergeDoc(ctx, patchDoc); err != nil {
			catcher.Wrap(err, "building GitHub merge queue patch document")
		}
	case patch.TriggerIntentType:
		patchedProject, patchedParserProject, err = j.buildTriggerPatchDoc(ctx, patchDoc)
		catcher.Wrap(err, "building trigger patch document")
	default:
		return errors.Errorf("intent type '%s' is unknown", j.IntentType)
	}

	if err = catcher.Resolve(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":      "failed to build patch document",
			"job":          j.ID(),
			"patch_id":     j.PatchID,
			"intent_type":  j.IntentType,
			"intent_id":    j.IntentID,
			"github_error": j.gitHubError,
			"source":       "patch intents",
		}))

		return err
	}

	if j.user == nil {
		j.user, err = user.FindOne(user.ById(patchDoc.Author))
		if err != nil {
			return errors.Wrapf(err, "finding patch author '%s'", patchDoc.Author)
		}
		if j.user == nil {
			return errors.Errorf("patch author '%s' not found", patchDoc.Author)
		}
	}

	if j.user.Settings.UseSpruceOptions.SpruceV1 {
		patchDoc.DisplayNewUI = true
	}

	pref, err := model.FindMergedProjectRef(patchDoc.Project, patchDoc.Version, true)
	if err != nil {
		return errors.Wrap(err, "finding project for patch")
	}
	if pref == nil {
		return errors.Errorf("project ref '%s' not found", patchDoc.Project)
	}

	// hidden projects can only run PR patches
	if !pref.Enabled && (j.IntentType != patch.GithubIntentType || !pref.IsHidden()) {
		j.gitHubError = ProjectDisabled
		return errors.New("project is disabled")
	}

	if pref.IsPatchingDisabled() {
		j.gitHubError = PatchingDisabled
		return errors.New("patching is disabled for project")
	}

	if patchDoc.IsBackport() && !pref.CommitQueue.IsEnabled() {
		j.gitHubError = commitQueueDisabled
		return errors.New("commit queue is disabled for project")
	}

	if !pref.TaskSync.IsPatchEnabled() && (len(patchDoc.SyncAtEndOpts.Tasks) != 0 || len(patchDoc.SyncAtEndOpts.BuildVariants) != 0) {
		j.gitHubError = PatchTaskSyncDisabled
		return errors.New("task sync at the end of a patched task is disabled by project settings")
	}

	if j.IntentType == patch.GithubIntentType && pref.OldestAllowedMergeBase != "" {
		isMergeBaseAllowed, err := thirdparty.IsMergeBaseAllowed(ctx, token, patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo, pref.OldestAllowedMergeBase, patchDoc.GithubPatchData.MergeBase)
		if err != nil {
			return errors.Wrap(err, "checking if merge base is allowed")
		}
		if !isMergeBaseAllowed {
			j.gitHubError = MergeBaseTooOld
			return errors.New("merge base is older than the oldest allowed merge base in project settings")
		}
	}

	validationCatcher := grip.NewBasicCatcher()
	// Get and validate patched config
	var patchedProjectConfig string
	if patchedParserProject != nil {
		patchedProjectConfig, err = model.GetPatchedProjectConfig(ctx, j.env.Settings(), patchDoc, token)
		if err != nil {
			return errors.Wrap(j.setGitHubPatchingError(err), "getting patched project config")
		}
	} else {
		var patchConfig *model.PatchConfig
		patchedProject, patchConfig, err = model.GetPatchedProject(ctx, j.env.Settings(), patchDoc, token)
		if err != nil {
			return errors.Wrap(j.setGitHubPatchingError(err), "getting patched project")
		}
		patchedParserProject = patchConfig.PatchedParserProject
		patchedProjectConfig = patchConfig.PatchedProjectConfig
	}
	if errs := validator.CheckProjectErrors(ctx, patchedProject, false).AtLevel(validator.Error); len(errs) != 0 {
		validationCatcher.Errorf("invalid patched config syntax: %s", validator.ValidationErrorsToString(errs))
	}
	if errs := validator.CheckProjectSettings(ctx, j.env.Settings(), patchedProject, pref, false).AtLevel(validator.Error); len(errs) != 0 {
		validationCatcher.Errorf("invalid patched config for current project settings: %s", validator.ValidationErrorsToString(errs))
	}

	if errs := validator.CheckPatchedProjectConfigErrors(patchedProjectConfig).AtLevel(validator.Error); len(errs) != 0 {
		validationCatcher.Errorf("invalid patched project config syntax: %s", validator.ValidationErrorsToString(errs))
	}
	if validationCatcher.HasErrors() {
		j.gitHubError = ProjectFailsValidation
		return errors.Wrapf(validationCatcher.Resolve(), "invalid patched project config")
	}
	// Don't create patches for github PRs if the only changes are in ignored files.
	if patchDoc.IsGithubPRPatch() && patchedProject.IgnoresAllFiles(patchDoc.FilesChanged()) {
		j.sendGitHubSuccessMessages(ctx, patchDoc, pref, ignoredFiles)
		return nil
	}

	patchDoc.PatchedProjectConfig = patchedProjectConfig

	for _, modulePatch := range patchDoc.Patches {
		if modulePatch.ModuleName != "" {
			// validate the module exists
			var module *model.Module
			module, err = patchedProject.GetModuleByName(modulePatch.ModuleName)
			if err != nil {
				return errors.Wrapf(err, "finding module '%s'", modulePatch.ModuleName)
			}
			if module == nil {
				return errors.Errorf("module '%s' not found", modulePatch.ModuleName)
			}
		}
	}
	if err = j.verifyValidAlias(pref.Id, patchDoc.PatchedProjectConfig); err != nil {
		j.gitHubError = invalidAlias
		return err
	}

	if err = j.buildTasksAndVariants(patchDoc, patchedProject); err != nil {
		return err
	}

	if (j.intent.ShouldFinalizePatch() || patchDoc.IsCommitQueuePatch()) &&
		len(patchDoc.VariantsTasks) == 0 {
		j.gitHubError = NoTasksOrVariants
		return errors.New("patch has no build variants or tasks")
	}

	if shouldTaskSync := len(patchDoc.SyncAtEndOpts.BuildVariants) != 0 || len(patchDoc.SyncAtEndOpts.Tasks) != 0; shouldTaskSync {
		patchDoc.SyncAtEndOpts.VariantsTasks = patchDoc.ResolveSyncVariantTasks(patchedProject.GetAllVariantTasks())
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
		patchDoc.Description = model.MakeCommitQueueDescription(patchDoc.Patches, pref, patchedProject, patchDoc.IsGithubMergePatch(), patchDoc.GithubMergeData)
	}

	if patchDoc.IsBackport() {
		patchDoc.Description, err = patchDoc.MakeBackportDescription()
		if err != nil {
			return errors.Wrap(err, "making backport patch description")
		}
	}

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = j.user.IncPatchNumber()
	if err != nil {
		return errors.Wrap(err, "computing patch number")
	}

	if patchDoc.CreateTime.IsZero() {
		patchDoc.CreateTime = time.Now()
	}
	// Set the new patch ID here because the ID for the patch created in this
	// job differs from the patch intent ID. Presumably, this is because the
	// patch intent and the actual patch are separate documents.
	patchDoc.Id = j.PatchID

	// Ensure that the patched parser project's ID agrees with the patch that's
	// about to be created, rather than the patch intent.
	patchedParserProject.Init(j.PatchID.Hex(), patchDoc.CreateTime)

	ppStorageMethod, err := model.ParserProjectUpsertOneWithS3Fallback(ctx, j.env.Settings(), evergreen.ProjectStorageMethodDB, patchedParserProject)
	if err != nil {
		return errors.Wrapf(err, "upserting parser project '%s' for patch", patchedParserProject.Id)
	}
	patchDoc.ProjectStorageMethod = ppStorageMethod

	if err = patchDoc.Insert(); err != nil {
		// If this is a duplicate key error, we already inserted the patch
		// in to the DB but it failed later in the patch intent job (i.e.
		// context cancelling early from deploy). To reduce stuck patches,
		// we continue on duplicate key errors. Since the workaround is a new
		// patch, GH merge queue patches getting stuck do not have an
		// easy workaround.
		if !mongo.IsDuplicateKeyError(err) {
			return errors.Wrapf(err, "inserting patch '%s'", patchDoc.Id.Hex())
		}
	}

	if err = ProcessTriggerAliases(ctx, patchDoc, pref, j.env, patchDoc.Triggers.Aliases); err != nil {
		if strings.Contains(err.Error(), noChildPatchTasksOrVariants) {
			j.gitHubError = noChildPatchTasksOrVariants
		}
		return errors.Wrap(err, "processing trigger aliases")
	}

	if patchDoc.IsGithubPRPatch() {
		numCheckRuns := patchedProject.GetNumCheckRunsFromVariantTasks(patchDoc.VariantsTasks)
		checkRunLimit := j.env.Settings().GitHubCheckRun.CheckRunLimit
		if numCheckRuns > checkRunLimit {
			return errors.Errorf("total number of checkRuns (%d) exceeds maximum limit (%d)", numCheckRuns, checkRunLimit)
		}
		catcher.Wrap(j.createGitHubSubscriptions(patchDoc), "creating GitHub PR patch subscriptions")
	}

	if patchDoc.IsGithubMergePatch() {
		catcher.Wrap(j.createGitHubMergeSubscription(ctx, patchDoc), "creating GitHub merge queue subscriptions")
	}
	if patchDoc.IsBackport() {
		backportSubscription := event.NewExpiringPatchSuccessSubscription(j.PatchID.Hex(), event.NewEnqueuePatchSubscriber())
		if err = backportSubscription.Upsert(); err != nil {
			catcher.Wrap(err, "inserting backport subscription")
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
		if _, err = model.FinalizePatch(ctx, patchDoc, j.intent.RequesterIdentity(), token); err != nil {
			if strings.Contains(err.Error(), thirdparty.Github502Error) {
				j.gitHubError = GitHubInternalError
			}
			grip.Error(message.WrapError(err, message.Fields{
				"message":     "failed to finalize patch document",
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

// setGitHubPatchingError sets the GitHub error message and returns it if
// loading the patched project errored.
func (j *patchIntentProcessor) setGitHubPatchingError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), model.EmptyConfigurationError) {
		j.gitHubError = EmptyConfig
	}
	if strings.Contains(err.Error(), thirdparty.Github502Error) {
		j.gitHubError = GitHubInternalError
	}
	if strings.Contains(err.Error(), model.LoadProjectError) {
		j.gitHubError = InvalidConfig
	}
	return err
}

// createGitHubSubscriptions creates subscriptions for notifications related to
// GitHub PR patches.
func (j *patchIntentProcessor) createGitHubSubscriptions(p *patch.Patch) error {
	catcher := grip.NewBasicCatcher()
	ghSub := event.NewGithubStatusAPISubscriber(event.GithubPullRequestSubscriber{
		Owner:    p.GithubPatchData.BaseOwner,
		Repo:     p.GithubPatchData.BaseRepo,
		PRNumber: p.GithubPatchData.PRNumber,
		Ref:      p.GithubPatchData.HeadHash,
	})
	patchSub := event.NewExpiringPatchOutcomeSubscription(j.PatchID.Hex(), ghSub)
	catcher.Wrap(patchSub.Upsert(), "inserting patch subscription for GitHub PR")
	buildSub := event.NewExpiringBuildOutcomeSubscriptionByVersion(j.PatchID.Hex(), ghSub)
	catcher.Wrap(buildSub.Upsert(), "inserting build subscription for GitHub PR")
	if p.IsParent() {
		// add a subscription on each child patch to report it's status to github when it's done.
		for _, childPatch := range p.Triggers.ChildPatches {
			childGhStatusSub := event.NewGithubStatusAPISubscriber(event.GithubPullRequestSubscriber{
				Owner:    p.GithubPatchData.BaseOwner,
				Repo:     p.GithubPatchData.BaseRepo,
				PRNumber: p.GithubPatchData.PRNumber,
				Ref:      p.GithubPatchData.HeadHash,
				ChildId:  childPatch,
			})
			patchSub := event.NewExpiringPatchChildOutcomeSubscription(childPatch, childGhStatusSub)
			catcher.Wrap(patchSub.Upsert(), "inserting child patch subscription for GitHub PR")
		}
	}
	return catcher.Resolve()
}

// createGithubMergeSubscription creates a subscription on a commit for the GitHub merge queue.
func (j *patchIntentProcessor) createGitHubMergeSubscription(ctx context.Context, p *patch.Patch) error {
	catcher := grip.NewBasicCatcher()
	ghSub := event.NewGithubMergeAPISubscriber(event.GithubMergeSubscriber{
		Owner: p.GithubMergeData.Org,
		Repo:  p.GithubMergeData.Repo,
		Ref:   p.GithubMergeData.HeadSHA,
	})

	patchSub := event.NewExpiringPatchOutcomeSubscription(j.PatchID.Hex(), ghSub)
	catcher.Wrap(patchSub.Upsert(), "inserting patch subscription for GitHub merge queue")
	buildSub := event.NewExpiringBuildOutcomeSubscriptionByVersion(j.PatchID.Hex(), ghSub)
	catcher.Wrap(buildSub.Upsert(), "inserting build subscription for GitHub merge queue")

	input := thirdparty.SendGithubStatusInput{
		VersionId: j.PatchID.Hex(),
		Owner:     p.GithubMergeData.Org,
		Repo:      p.GithubMergeData.Repo,
		Ref:       p.GithubMergeData.HeadSHA,
		Desc:      "patch created",
		Caller:    j.Name,
	}

	rules := j.getEvergreenBranchProtectionRulesForStatuses(ctx, p.GithubMergeData.Org, p.GithubMergeData.Repo, p.GithubMergeData.BaseBranch)
	for i, rule := range rules {
		// Limit statuses to 10
		if i >= 10 {
			break
		}
		input.Context = rule
		catcher.Wrap(thirdparty.SendPendingStatusToGithub(ctx, input, j.env.Settings().Ui.Url), "failed to send pending status to GitHub")
	}

	return catcher.Resolve()
}

func (j *patchIntentProcessor) buildTasksAndVariants(patchDoc *patch.Patch, project *model.Project) error {
	var err error
	var reuseDef bool
	reusePatchId, failedOnly := j.intent.RepeatFailedTasksAndVariants()
	if !failedOnly {
		reusePatchId, reuseDef = j.intent.RepeatPreviousPatchDefinition()
	}

	if reuseDef || failedOnly {
		err = j.setToPreviousPatchDefinition(patchDoc, project, reusePatchId, failedOnly)
		if err != nil {
			return err
		}
		if j.IntentType == patch.GithubIntentType {
			patchDoc.GithubPatchData.RepeatPatchIdNextPatch = reusePatchId
		}
		return nil
	}

	// Verify that all variants exists
	for _, buildVariant := range patchDoc.BuildVariants {
		if buildVariant == "all" || buildVariant == "" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			return errors.Errorf("no such buildvariant matching '%s'", buildVariant)
		}
	}

	for _, bv := range patchDoc.RegexBuildVariants {
		_, err := regexp.Compile(bv)
		if err != nil {
			return errors.Wrapf(err, "compiling buildvariant regex '%s'", bv)
		}
	}
	for _, t := range patchDoc.RegexTasks {
		_, err := regexp.Compile(t)
		if err != nil {
			return errors.Wrapf(err, "compiling task regex '%s'", t)
		}
	}

	if len(patchDoc.VariantsTasks) == 0 {
		project.BuildProjectTVPairs(patchDoc, j.intent.GetAlias())
	}
	return nil
}

// setToFilteredTasks sets the tasks/variants to a previous patch's activated tasks (filtered on failures if requested)
// and adds dependencies and task group tasks as needed.
func setToFilteredTasks(patchDoc, reusePatch *patch.Patch, project *model.Project, failedOnly bool) error {
	activatedTasks, err := task.FindActivatedByVersionWithoutDisplay(reusePatch.Version)
	if err != nil {
		return errors.Wrap(err, "filtering to activated tasks")
	}

	activatedTasksDisplayNames := []string{}
	failedTaskDisplayNames := []string{}
	failedTasks := []task.Task{}
	for _, t := range activatedTasks {
		activatedTasksDisplayNames = append(activatedTasksDisplayNames, t.DisplayName)
		if failedOnly && evergreen.IsFailedTaskStatus(t.Status) {
			failedTasks = append(failedTasks, t)
			failedTaskDisplayNames = append(failedTaskDisplayNames, t.DisplayName)
		}
	}
	filteredTasks := activatedTasksDisplayNames
	if failedOnly {
		filteredTasks = failedTaskDisplayNames
	}

	filteredVariantTasks := []patch.VariantTasks{}
	for _, vt := range reusePatch.VariantsTasks {
		// Limit it to tasks that are failed or who have failed tasks depending on them.
		// We only need to add dependencies and task group tasks for failed tasks because otherwise
		// we can rely on them being there from the previous patch.
		if failedOnly {
			failedPlusNeeded, err := addDependenciesAndTaskGroups(failedTasks, failedTaskDisplayNames, project, vt)
			if err != nil {
				return errors.Wrap(err, "getting dependencies and task groups for activated tasks")
			}
			filteredTasks = append(filteredTasks, failedPlusNeeded...)
		}

		variantTask := vt
		variantTask.Tasks = utility.StringSliceIntersection(filteredTasks, vt.Tasks)

		// only add build variants and variant tasks if there are tasks in them that are being reused
		if len(variantTask.Tasks) != 0 || len(variantTask.DisplayTasks) != 0 {
			filteredVariantTasks = append(filteredVariantTasks, variantTask)
			patchDoc.BuildVariants = append(patchDoc.BuildVariants, vt.Variant)
		}

	}

	patchDoc.Tasks = filteredTasks
	patchDoc.VariantsTasks = filteredVariantTasks

	return nil
}

// addDependenciesAndTaskGroups adds dependencies and tasks from single host task groups for the given tasks.
func addDependenciesAndTaskGroups(tasks []task.Task, taskDisplayNames []string, project *model.Project, vt patch.VariantTasks) ([]string, error) {
	// only add tasks if they are in the current project definition
	tasksInProjectVariant := project.FindTasksForVariant(vt.Variant)
	tasksToAdd := []string{}
	// add dependencies of failed tasks
	taskDependencies, err := task.GetRecursiveDependenciesUp(tasks, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting dependencies for activated tasks")
	}
	for _, t := range taskDependencies {
		if utility.StringSliceContains(tasksInProjectVariant, t.DisplayName) && !utility.StringSliceContains(taskDisplayNames, t.DisplayName) {
			tasksToAdd = append(tasksToAdd, t.DisplayName)
		}
	}

	for _, t := range tasks {
		// Schedule all tasks in a single host task group because they may need to execute together to order to succeed.
		if utility.StringSliceContains(tasksInProjectVariant, t.TaskGroup) && t.TaskGroup != "" && t.IsPartOfSingleHostTaskGroup() {
			taskGroup := project.FindTaskGroup(t.TaskGroup)
			for _, t := range taskGroup.Tasks {
				if !utility.StringSliceContains(taskDisplayNames, t) {
					tasksToAdd = append(tasksToAdd, t)
				}
			}
		}
	}

	return tasksToAdd, nil

}

// setToPreviousPatchDefinition sets the tasks/variants based on a previous patch.
// If failedOnly is set, we only use the tasks/variants that failed.
// If patchId isn't set, we just use the most recent patch for the project.
func (j *patchIntentProcessor) setToPreviousPatchDefinition(patchDoc *patch.Patch,
	project *model.Project, patchId string, failedOnly bool) error {
	var reusePatch *patch.Patch
	var err error
	if patchId == "" {
		reusePatch, err = patch.FindOne(patch.MostRecentPatchByUserAndProject(j.user.Username(), project.Identifier))
		if err != nil {
			return errors.Wrap(err, "querying for most recent patch")
		}
		if reusePatch == nil {
			return errors.Errorf("no previous patch available")
		}
	} else {
		reusePatch, err = patch.FindOneId(patchId)
		if err != nil {
			return errors.Wrapf(err, "querying for patch '%s'", patchId)
		}
		if reusePatch == nil {
			return errors.Errorf("patch '%s' not found", patchId)
		}
	}

	if j.IntentType == patch.GithubIntentType {
		patchDoc.Tasks = reusePatch.Tasks
		patchDoc.BuildVariants = reusePatch.BuildVariants
		patchDoc.VariantsTasks = reusePatch.VariantsTasks
		return nil
	}

	if err = setToFilteredTasks(patchDoc, reusePatch, project, failedOnly); err != nil {
		return errors.Wrapf(err, "filtering tasks for '%s'", patchId)
	}

	return nil
}

func ProcessTriggerAliases(ctx context.Context, p *patch.Patch, projectRef *model.ProjectRef, env evergreen.Environment, aliasNames []string) error {
	if len(aliasNames) == 0 {
		return nil
	}

	type aliasGroup struct {
		project            string
		status             string
		parentAsModule     string
		downstreamRevision string
	}
	aliasGroups := make(map[aliasGroup][]patch.PatchTriggerDefinition)
	for _, aliasName := range aliasNames {
		alias, found := projectRef.GetPatchTriggerAlias(aliasName)
		if !found {
			return errors.Errorf("patch trigger alias '%s' is not defined", aliasName)
		}
		// group patches on project, status, parentAsModule, and revision
		group := aliasGroup{
			project:            alias.ChildProject,
			status:             alias.Status,
			parentAsModule:     alias.ParentAsModule,
			downstreamRevision: alias.DownstreamRevision,
		}
		aliasGroups[group] = append(aliasGroups[group], alias)
	}

	triggerIntents := make([]patch.Intent, 0, len(aliasGroups))
	for group, definitions := range aliasGroups {
		triggerIntent := patch.NewTriggerIntent(patch.TriggerIntentOptions{
			ParentID:           p.Id.Hex(),
			ParentProjectID:    p.Project,
			ParentStatus:       group.status,
			ProjectID:          group.project,
			ParentAsModule:     group.parentAsModule,
			DownstreamRevision: group.downstreamRevision,
			Requester:          p.GetRequester(),
			Author:             p.Author,
			Definitions:        definitions,
		})

		if err := triggerIntent.Insert(); err != nil {
			return errors.Wrap(err, "inserting trigger intent")
		}

		triggerIntents = append(triggerIntents, triggerIntent)
		p.Triggers.ChildPatches = append(p.Triggers.ChildPatches, triggerIntent.ID())
	}
	if err := p.SetChildPatches(); err != nil {
		return errors.Wrap(err, "setting child patch IDs")
	}

	for _, intent := range triggerIntents {
		triggerIntent, ok := intent.(*patch.TriggerIntent)
		if !ok {
			return errors.Errorf("intent '%s' didn't not have expected type '%T'", intent.ID(), intent)
		}

		job := NewPatchIntentProcessor(env, mgobson.ObjectIdHex(intent.ID()), intent)
		if triggerIntent.ParentStatus == "" {
			// In order to be able to finalize a patch from the CLI,
			// we need the child patch intents to exist when the parent patch is finalized.
			job.Run(ctx)
			if err := job.Error(); err != nil {
				return errors.Wrap(err, "processing child patch")
			}
		} else {
			if err := env.RemoteQueue().Put(ctx, job); err != nil {
				return errors.Wrap(err, "enqueueing child patch processing")
			}
		}
	}

	return nil
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
		return errors.Wrapf(err, "finding project ref '%s'", patchDoc.Project)
	}
	if projectRef == nil {
		return errors.Errorf("project ref '%s' not found", patchDoc.Project)
	}

	if patchDoc.IsBackport() {
		return j.buildBackportPatchDoc(ctx, projectRef, patchDoc)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	commit, err := thirdparty.GetCommitEvent(ctx, githubOauthToken, projectRef.Owner,
		projectRef.Repo, patchDoc.Githash)
	if err != nil {
		return errors.Wrapf(err, "finding base revision '%s' for project '%s'",
			patchDoc.Githash, projectRef.Id)
	}
	// With `evergreen patch-file`, a user can pass a branch name or tag instead of a hash. We
	// must normalize this to a hash before storing the patch doc.
	if commit != nil && commit.SHA != nil && patchDoc.Githash != *commit.SHA {
		patchDoc.Githash = *commit.SHA
	}

	if len(patchDoc.Patches) > 0 {
		if patchDoc.Patches[0], err = getModulePatch(patchDoc.Patches[0]); err != nil {
			return errors.Wrap(err, "getting module patch from GridFS")
		}
	}

	return nil
}

// getModulePatch reads the patch from GridFS, processes it, and
// stores the resulting summaries in the returned ModulePatch
func getModulePatch(modulePatch patch.ModulePatch) (patch.ModulePatch, error) {
	patchContents, err := patch.FetchPatchContents(modulePatch.PatchSet.PatchFileId)
	if err != nil {
		return modulePatch, errors.Wrap(err, "fetching patch contents")
	}

	var summaries []thirdparty.Summary
	if patch.IsMailboxDiff(patchContents) {
		var commitMessages []string
		summaries, commitMessages, err = thirdparty.GetPatchSummariesFromMboxPatch(patchContents)
		if err != nil {
			return modulePatch, errors.Wrapf(err, "getting patch summaries by commit")
		}
		modulePatch.PatchSet.CommitMessages = commitMessages
	} else {
		summaries, err = thirdparty.GetPatchSummaries(patchContents)
		if err != nil {
			return modulePatch, errors.Wrap(err, "getting patch summaries")
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
			return errors.Wrap(err, "getting existing merge patch")
		}
		if existingMergePatch == nil {
			return errors.Errorf("patch '%s' not found", patchDoc.BackportOf.PatchID)
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
		return errors.Wrapf(err, "creating a patch set for SHA '%s'", patchDoc.BackportOf.SHA)
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
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return false, errors.Wrap(err, "checking if GitHub PR testing is disabled")
	}
	if flags.GithubPRTestingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     patchIntentJobName,
			"message": "GitHub PR testing is disabled, not processing pull request",

			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
		})
		return false, errors.New("not processing PR because GitHub PR testing is disabled")
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
		return false, errors.New("GitHub PR testing is not configured correctly because it requires a GitHub org to authenticate against")
	}

	projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
		patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch, j.intent.GetCalledBy())
	if err != nil {
		return false, errors.Wrapf(err, "fetching project ref for repo '%s/%s' with branch '%s'",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.BaseBranch)
	}
	if projectRef == nil {
		return false, errors.Errorf("project ref for repo '%s/%s' with branch '%s' not found",
			patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.BaseBranch)
	}

	if len(projectRef.GithubTriggerAliases) > 0 {
		patchDoc.Triggers = patch.TriggerInfo{Aliases: projectRef.GithubTriggerAliases}
	}

	isMember, err := j.isUserAuthorized(ctx, patchDoc, mustBeMemberOfOrg,
		patchDoc.GithubPatchData.Author, githubOauthToken)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":     "GitHub API failure",
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
	} else if !isMember {
		grip.Debug(message.Fields{
			"message":     "user unauthorized to start patch",
			"user":        patchDoc.GithubPatchData.Author,
			"source":      "patch intents",
			"job":         j.ID(),
			"patch_id":    j.PatchID,
			"pr_number":   patchDoc.GithubPatchData.PRNumber,
			"head_repo":   fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"intent_type": j.IntentType,
			"intent_id":   j.IntentID,
		})
	}

	j.user, err = findEvergreenUserForPR(patchDoc.GithubPatchData.AuthorUID)
	if err != nil {
		return isMember, errors.Wrapf(err, "finding user associated with GitHub UID '%d'", patchDoc.GithubPatchData.AuthorUID)
	}
	patchDoc.Author = j.user.Id
	patchDoc.Project = projectRef.Id

	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(ctx, githubOauthToken, patchDoc.GithubPatchData)
	if err != nil {
		// Expected error when the PR diff is more than 3000 lines or 300 files.
		if strings.Contains(err.Error(), thirdparty.PRDiffTooLargeErrorMessage) {
			return isMember, nil
		}
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

	if err = db.WriteGridFile(patch.GridFSPrefix, patchFileID, strings.NewReader(patchContent)); err != nil {
		return isMember, errors.Wrap(err, "writing patch file to DB")
	}

	return isMember, nil
}

func (j *patchIntentProcessor) buildGithubMergeDoc(ctx context.Context, patchDoc *patch.Patch) error {
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

	projectRef, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(patchDoc.GithubMergeData.Org,
		patchDoc.GithubMergeData.Repo, patchDoc.GithubMergeData.BaseBranch)
	if err != nil {
		return errors.Wrapf(err, "fetching project ref for repo '%s/%s' with branch '%s'",
			patchDoc.GithubMergeData.Org, patchDoc.GithubMergeData.Repo, patchDoc.GithubMergeData.BaseBranch,
		)
	}
	if projectRef == nil {
		j.gitHubError = commitQueueDisabled
		return errors.Errorf("project ref for repo '%s/%s' with branch '%s' and merge queue enabled not found",
			patchDoc.GithubMergeData.Org, patchDoc.GithubMergeData.Repo, patchDoc.GithubMergeData.BaseBranch)
	}
	j.user, err = findEvergreenUserForGithubMergeGroup(patchDoc.GithubPatchData.AuthorUID)
	if err != nil {
		return errors.Wrap(err, "finding GitHub merge queue user")
	}
	patchDoc.Author = j.user.Id
	patchDoc.Project = projectRef.Id

	return nil
}

func (j *patchIntentProcessor) buildTriggerPatchDoc(ctx context.Context, patchDoc *patch.Patch) (*model.Project, *model.ParserProject, error) {
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
		return nil, nil, errors.Errorf("programmatic error: expected intent '%s' to be a trigger intent type but instead got '%T'", j.IntentID, j.intent)
	}
	v, project, pp, err := fetchTriggerVersionInfo(ctx, patchDoc)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "getting latest version for project '%s'", patchDoc.Project)
	}

	patchDoc.Githash = v.Revision
	matchingTasks, err := project.VariantTasksForSelectors(intent.Definitions, patchDoc.GetRequester())
	if err != nil {
		return nil, nil, errors.Wrap(err, "matching tasks to alias definitions")
	}
	if len(matchingTasks) == 0 {
		// Adding to Github error here directly doesn't work, since we need the parent patch to send the
		// error to the Github PR, so instead we return it as an error that we case on.
		return nil, nil, errors.New(noChildPatchTasksOrVariants)
	}

	patchDoc.VariantsTasks = matchingTasks
	if intent.ParentAsModule != "" || patchDoc.Triggers.SameBranchAsParent {
		parentPatch, err := patch.FindOneId(patchDoc.Triggers.ParentPatch)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting parent patch '%s'", patchDoc.Triggers.ParentPatch)
		}
		if parentPatch == nil {
			return nil, nil, errors.Errorf("parent patch '%s' not found", patchDoc.Triggers.ParentPatch)
		}
		for _, p := range parentPatch.Patches {
			if p.ModuleName == "" {
				moduleName := intent.ParentAsModule
				if patchDoc.Triggers.SameBranchAsParent {
					// If the parent patch uses the same repo and branch as the child project,
					// make the child patch use the same revision and patches as the parent patch.
					patchDoc.Githash = parentPatch.Githash
					moduleName = ""
				}
				patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{
					// Apply the parent patch's changes if both child and parent are using the
					// same repo/project/branch
					ModuleName: moduleName,
					PatchSet:   p.PatchSet,
					Githash:    parentPatch.Githash,
				})
				break
			}
		}
	}
	return project, pp, nil
}

func fetchTriggerVersionInfo(ctx context.Context, patchDoc *patch.Patch) (*model.Version, *model.Project, *model.ParserProject, error) {
	if patchDoc.Triggers.DownstreamRevision != "" {
		v, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(patchDoc.Project, patchDoc.Triggers.DownstreamRevision))
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "getting version at revision '%s'", patchDoc.Triggers.DownstreamRevision)
		}
		if v == nil {
			return nil, nil, nil, errors.Errorf("version at revision '%s' not found", patchDoc.Triggers.DownstreamRevision)
		}
		project, pp, err := model.FindAndTranslateProjectForVersion(ctx, evergreen.GetEnvironment().Settings(), v, true)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "getting downstream version at revision '%s' to use for patch '%s'", patchDoc.Triggers.DownstreamRevision, patchDoc.Id.Hex())
		}
		return v, project, pp, nil
	}
	v, project, pp, err := model.FindLatestVersionWithValidProject(patchDoc.Project, true)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "getting downstream version to use for patch '%s'", patchDoc.Id.Hex())
	}
	return v, project, pp, nil
}

func (j *patchIntentProcessor) verifyValidAlias(projectId string, configStr string) error {
	alias := j.intent.GetAlias()
	if alias == "" {
		return nil
	}
	var projectConfig *model.ProjectConfig
	if configStr != "" {
		var err error
		projectConfig, err = model.CreateProjectConfig([]byte(configStr), "")
		if err != nil {
			return errors.Wrap(err, "creating project config")
		}
	}
	aliases, err := model.FindAliasInProjectRepoOrProjectConfig(projectId, alias, projectConfig)
	if err != nil {
		return errors.Wrapf(err, "retrieving aliases for project '%s'", projectId)
	}
	if len(aliases) > 0 {
		return nil
	}
	return errors.Errorf("alias '%s' could not be found on project '%s'", alias, projectId)
}

func findEvergreenUserForPR(githubUID int) (*user.DBUser, error) {
	// try and find a user by GitHub UID
	u, err := user.FindByGithubUID(githubUID)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return u, nil
	}

	// Otherwise, use the GitHub patch user
	u, err = user.FindOne(user.ById(evergreen.GithubPatchUser))
	if err != nil {
		return u, errors.Wrap(err, "finding GitHub patch user")
	}
	// and if that user doesn't exist, make it
	if u == nil {
		u = &user.DBUser{
			Id:       evergreen.GithubPatchUser,
			DispName: "GitHub Pull Requests",
			APIKey:   utility.RandomString(),
		}
		if err = u.Insert(); err != nil {
			return nil, errors.Wrap(err, "inserting GitHub patch user")
		}
	}

	return u, err
}

func findEvergreenUserForGithubMergeGroup(githubUID int) (*user.DBUser, error) {
	u, err := user.FindOne(user.ById(evergreen.GithubMergeUser))
	if err != nil {
		return u, errors.Wrap(err, "finding GitHub merge queue user")
	}
	// and if that user doesn't exist, make it
	if u == nil {
		u = &user.DBUser{
			Id:       evergreen.GithubMergeUser,
			DispName: "GitHub Merge Queue",
			APIKey:   utility.RandomString(),
		}
		if err = u.Insert(); err != nil {
			return nil, errors.Wrap(err, "inserting GitHub patch user")
		}
	}

	return u, err
}

func (j *patchIntentProcessor) isUserAuthorized(ctx context.Context, patchDoc *patch.Patch, requiredOrganization, githubUser, githubOauthToken string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// GitHub Dependabot patches should be automatically authorized.
	if githubUser == githubDependabotUser {
		grip.Info(message.Fields{
			"job":       j.ID(),
			"message":   fmt.Sprintf("authorizing patch from special user '%s'", githubDependabotUser),
			"source":    "patch intents",
			"base_repo": fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo": fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number": patchDoc.GithubPatchData.PRNumber,
		})
		return true, nil
	}
	// Checking if the GitHub user is in the organization is more permissive than checking permission level
	// for the owner/repo specified, however this is okay since for the purposes of this check its to run patches.
	isMember, err := thirdparty.GithubUserInOrganization(ctx, githubOauthToken, requiredOrganization, githubUser)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job":          j.ID(),
			"message":      "failed to authenticate GitHub PR",
			"source":       "patch intents",
			"creator":      githubUser,
			"required_org": requiredOrganization,
			"base_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":    patchDoc.GithubPatchData.PRNumber,
		}))
		return false, err
	}
	if isMember {
		return isMember, nil
	}

	isAuthorizedForOrg, err := thirdparty.AppAuthorizedForOrg(ctx, githubOauthToken, requiredOrganization, githubUser)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job":          j.ID(),
			"message":      "failed to check if user is an authorized app",
			"source":       "patch intents",
			"creator":      githubUser,
			"required_org": requiredOrganization,
			"base_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.BaseRepo),
			"head_repo":    fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":    patchDoc.GithubPatchData.PRNumber,
		}))
	}
	if isAuthorizedForOrg {
		return isAuthorizedForOrg, nil
	}

	// Verify external collaborators separately.
	hasWritePermission, err := thirdparty.GitHubUserHasWritePermission(ctx, githubOauthToken,
		patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo, githubUser)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"job":        j.ID(),
			"message":    "failed to check if user has write permission for repo",
			"source":     "patch intents",
			"creator":    githubUser,
			"head_owner": fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.BaseOwner, patchDoc.GithubPatchData.HeadOwner),
			"head_repo":  fmt.Sprintf("%s/%s", patchDoc.GithubPatchData.HeadOwner, patchDoc.GithubPatchData.HeadRepo),
			"pr_number":  patchDoc.GithubPatchData.PRNumber,
		}))
	}
	return hasWritePermission, nil
}

func (j *patchIntentProcessor) sendGitHubErrorStatus(ctx context.Context, patchDoc *patch.Patch) {
	var update amboy.Job
	if j.IntentType == patch.GithubIntentType {
		update = NewGithubStatusUpdateJobForProcessingError(
			thirdparty.GithubStatusDefaultContext,
			patchDoc.GithubPatchData.BaseOwner,
			patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.HeadHash,
			j.gitHubError,
		)
	} else if j.IntentType == patch.GithubMergeIntentType {
		update = NewGithubStatusUpdateJobForProcessingError(
			thirdparty.GithubStatusDefaultContext,
			patchDoc.GithubMergeData.Org,
			patchDoc.GithubMergeData.Repo,
			patchDoc.GithubMergeData.HeadSHA,
			j.gitHubError,
		)
	} else {
		j.AddError(errors.Errorf("unexpected intent type '%s'", j.IntentType))
		return
	}
	update.Run(ctx)

	j.AddError(update.Error())
}

// sendGitHubSuccessMessages sends a successful status to GitHub with the given message for all
// branch protection rules configured for the given project.
func (j *patchIntentProcessor) sendGitHubSuccessMessages(ctx context.Context, patchDoc *patch.Patch, projectRef *model.ProjectRef, msg string) {
	rules := j.getEvergreenBranchProtectionRulesForStatuses(ctx, patchDoc.GithubPatchData.BaseOwner, projectRef.Repo, projectRef.Branch)
	for _, rule := range rules {
		update := NewGithubStatusUpdateJobWithSuccessMessage(
			rule,
			patchDoc.GithubPatchData.BaseOwner,
			patchDoc.GithubPatchData.BaseRepo,
			patchDoc.GithubPatchData.HeadHash,
			msg,
		)
		update.Run(ctx)
		j.AddError(update.Error())
	}
}

// getEvergreenBranchProtectionRulesForStatuses returns the rules we want to send Evergreen statuses for.
// If we don't find rules, we'll send the status default context. We log the error but don't
// return it, because we might have permission to send statuses but not to get branch protection rules.
func (j *patchIntentProcessor) getEvergreenBranchProtectionRulesForStatuses(ctx context.Context, owner, repo, branch string) []string {
	rules, err := thirdparty.GetEvergreenBranchProtectionRules(ctx, "", owner, repo, branch)
	grip.Error(message.WrapError(err, message.Fields{
		"job":      j.ID(),
		"job_type": j.Type,
		"message":  "failed to get branch protection rules",
		"org":      owner,
		"repo":     repo,
		"branch":   branch,
		"patch":    j.PatchID.Hex(),
	}))

	return utility.UniqueStrings(append(rules, thirdparty.GithubStatusDefaultContext))
}
