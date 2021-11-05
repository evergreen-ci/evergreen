package model

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	mgobson "gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

type TaskVariantPairs struct {
	ExecTasks    TVPairSet
	DisplayTasks TVPairSet
}

// VariantTasksToTVPairs takes a set of variants and tasks (from both the old and new
// request formats) and builds a universal set of pairs
// that can be used to expand the dependency tree.
func VariantTasksToTVPairs(in []patch.VariantTasks) TaskVariantPairs {
	out := TaskVariantPairs{}
	for _, vt := range in {
		for _, t := range vt.Tasks {
			out.ExecTasks = append(out.ExecTasks, TVPair{vt.Variant, t})
		}
		for _, dt := range vt.DisplayTasks {
			out.DisplayTasks = append(out.DisplayTasks, TVPair{vt.Variant, dt.Name})
			for _, et := range dt.ExecTasks {
				out.ExecTasks = append(out.ExecTasks, TVPair{vt.Variant, et})
			}
		}
	}
	return out
}

// TVPairsToVariantTasks takes a list of TVPairs (task/variant pairs), groups the tasks
// for the same variant together under a single list, and returns all the variant groups
// as a set of patch.VariantTasks.
func (tvp *TaskVariantPairs) TVPairsToVariantTasks() []patch.VariantTasks {
	vtMap := map[string]patch.VariantTasks{}
	for _, pair := range tvp.ExecTasks {
		vt := vtMap[pair.Variant]
		vt.Variant = pair.Variant
		vt.Tasks = append(vt.Tasks, pair.TaskName)
		vtMap[pair.Variant] = vt
	}
	for _, dt := range tvp.DisplayTasks {
		variant := vtMap[dt.Variant]
		variant.Variant = dt.Variant
		variant.DisplayTasks = append(variant.DisplayTasks, patch.DisplayTask{Name: dt.TaskName})
		vtMap[dt.Variant] = variant
	}
	vts := []patch.VariantTasks{}
	for _, vt := range vtMap {
		vts = append(vts, vt)
	}
	return vts
}

// ValidateTVPairs checks that all of a set of variant/task pairs exist in a given project.
func ValidateTVPairs(p *Project, in []TVPair) error {
	for _, pair := range in {
		v := p.FindTaskForVariant(pair.TaskName, pair.Variant)
		if v == nil {
			return errors.Errorf("does not exist: task %v, variant %v", pair.TaskName, pair.Variant)
		}
	}
	return nil
}

// Given a patch version and a list of variant/task pairs, creates the set of new builds that
// do not exist yet out of the set of pairs. No tasks are added for builds which already exist
// (see AddNewTasksForPatch).
func AddNewBuildsForPatch(ctx context.Context, p *patch.Patch, patchVersion *Version, project *Project,
	tasks TaskVariantPairs, pRef *ProjectRef) error {
	_, err := addNewBuilds(ctx, specificActivationInfo{}, patchVersion, project, tasks, p.SyncAtEndOpts, pRef, "")
	return errors.Wrap(err, "can't add new builds")
}

// Given a patch version and set of variant/task pairs, creates any tasks that don't exist yet,
// within the set of already existing builds.
func AddNewTasksForPatch(ctx context.Context, p *patch.Patch, patchVersion *Version, project *Project,
	pairs TaskVariantPairs, projectIdentifier string) error {
	_, err := addNewTasks(ctx, specificActivationInfo{}, patchVersion, project, pairs, p.SyncAtEndOpts, projectIdentifier, "")
	return errors.Wrap(err, "can't add new tasks")
}

// GetPatchedProject creates and validates a project created by fetching latest commit information from GitHub
// and applying the patch to the latest remote configuration. Also returns the condensed yaml string for storage.
// The error returned can be a validation error.
func GetPatchedProject(ctx context.Context, p *patch.Patch, githubOauthToken string) (*Project, string, error) {
	if p.Version != "" {
		return nil, "", errors.Errorf("Patch %v already finalized", p.Version)
	}

	project := &Project{}
	projectRef, opts, err := getLoadProjectOptsForPatch(p, githubOauthToken)
	if err != nil {
		return nil, "", errors.Wrap(err, "Error fetching project opts for patch")
	}
	// if the patched config exists, use that as the project file bytes.
	if p.PatchedConfig != "" {
		if _, err := LoadProjectInto(ctx, []byte(p.PatchedConfig), opts, p.Project, project); err != nil {
			return nil, "", errors.WithStack(err)
		}
		return project, p.PatchedConfig, nil
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	env := evergreen.GetEnvironment()

	// try to get the remote project file data at the requested revision
	var projectFileBytes []byte
	hash := p.Githash
	if p.IsGithubPRPatch() {
		hash = p.GithubPatchData.HeadHash
	}
	if p.IsPRMergePatch() {
		hash = p.GithubPatchData.MergeCommitSHA
	}
	opts.Revision = hash

	path := projectRef.RemotePath
	if p.Path != "" && !p.IsGithubPRPatch() && !p.IsCommitQueuePatch() {
		path = p.Path
	}
	opts.RemotePath = path
	opts.PatchOpts.env = env
	projectFileBytes, err = getFileForPatchDiff(ctx, *opts)
	if err != nil {
		return nil, "", errors.Wrapf(err, "could not fetch remote configuration file")
	}

	// apply remote configuration patch if needed
	if !(p.IsGithubPRPatch() || p.IsPRMergePatch()) && p.ConfigChanged(path) {
		opts.ReadFileFrom = ReadFromPatchDiff
		projectFileBytes, err = MakePatchedConfig(ctx, env, p, path, string(projectFileBytes))
		if err != nil {
			return nil, "", errors.Wrapf(err, "Could not patch remote configuration file")
		}
	}
	pp, err := LoadProjectInto(ctx, projectFileBytes, opts, p.Project, project)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}

	out, err := yaml.Marshal(pp)
	if err != nil {
		return nil, "", errors.Wrapf(err, "Could not marshal parser project into yaml")
	}
	return project, string(out), nil
}

// MakePatchedConfig takes in the path to a remote configuration a stringified version
// of the current project and returns an unmarshalled version of the project
// with the patch applied
func MakePatchedConfig(ctx context.Context, env evergreen.Environment, p *patch.Patch, remoteConfigPath, projectConfig string) ([]byte, error) {
	for _, patchPart := range p.Patches {
		// we only need to patch the main project and not any other modules
		if patchPart.ModuleName != "" {
			continue
		}

		var patchFilePath string
		var err error
		if patchPart.PatchSet.Patch == "" {
			var patchContents string
			patchContents, err = patch.FetchPatchContents(patchPart.PatchSet.PatchFileId)
			if err != nil {
				return nil, errors.Wrap(err, "can't fetch patch contents")
			}
			patchFilePath, err = util.WriteToTempFile(patchContents)
			if err != nil {
				return nil, errors.Wrap(err, "could not write temporary patch file")
			}
		} else {
			patchFilePath, err = util.WriteToTempFile(patchPart.PatchSet.Patch)
			if err != nil {
				return nil, errors.Wrap(err, "could not write temporary patch file")
			}
		}

		defer os.Remove(patchFilePath) //nolint: evg-lint
		// write project configuration
		configFilePath, err := util.WriteToTempFile(projectConfig)
		if err != nil {
			return nil, errors.Wrap(err, "could not write config file")
		}
		defer os.Remove(configFilePath) //nolint: evg-lint

		// clean the working directory
		workingDirectory := filepath.Dir(patchFilePath)
		localConfigPath := filepath.Join(
			workingDirectory,
			remoteConfigPath,
		)
		parentDir := strings.Split(
			remoteConfigPath,
			string(os.PathSeparator),
		)[0]
		err = os.RemoveAll(filepath.Join(workingDirectory, parentDir))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if err = os.MkdirAll(filepath.Dir(localConfigPath), 0755); err != nil {
			return nil, errors.WithStack(err)
		}
		// rename the temporary config file name to the remote config
		// file path if we are patching an existing remote config
		if len(projectConfig) > 0 {
			if err = os.Rename(configFilePath, localConfigPath); err != nil {
				return nil, errors.Wrapf(err, "could not rename file '%v' to '%v'",
					configFilePath, localConfigPath)
			}
			defer os.Remove(localConfigPath)
		}

		// selectively apply the patch to the config file
		patchCommandStrings := []string{
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("git apply --whitespace=fix --include=%v < '%v'",
				remoteConfigPath, patchFilePath),
		}

		err = env.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", strings.Join(patchCommandStrings, "\n")}).
			SetErrorSender(level.Error, grip.GetSender()).SetOutputSender(level.Info, grip.GetSender()).
			Directory(workingDirectory).Run(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not run patch command possibly due to merge conflict on evergreen configuration file")
		}

		// read in the patched config file
		data, err := ioutil.ReadFile(localConfigPath)
		if err != nil {
			return nil, errors.Wrap(err, "could not read patched config file")
		}
		if string(data) == "" {
			return nil, errors.New(EmptyConfigurationError)
		}
		return data, nil
	}
	return nil, errors.New("no patch on project")
}

// Finalizes a patch:
// Patches a remote project's configuration file if needed.
// Creates a version for this patch and links it.
// Creates builds based on the Version
func FinalizePatch(ctx context.Context, p *patch.Patch, requester string, githubOauthToken string) (*Version, error) {
	// unmarshal the project YAML for storage
	project := &Project{}
	projectRef, opts, err := getLoadProjectOptsForPatch(p, githubOauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "Error fetching project opts for patch")
	}
	intermediateProject, err := LoadProjectInto(ctx, []byte(p.PatchedConfig), opts, p.Project, project)
	if err != nil {
		return nil, errors.Wrapf(err,
			"Error marshaling patched project config from repository revision “%v”",
			p.Githash)
	}
	intermediateProject.Id = p.Id.Hex()

	distroAliases, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return nil, errors.Wrap(err, "problem resolving distro alias table for patch")
	}

	githubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = thirdparty.GetCommitEvent(githubCtx, githubOauthToken, projectRef.Owner, projectRef.Repo, p.Githash)
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't fetch commit information")
	}

	var parentPatchNumber int
	if p.IsChild() {
		parentPatch, err := p.SetParametersFromParent()
		if err != nil {
			return nil, errors.Wrap(err, "problem getting parameters from parent patch")
		}
		parentPatchNumber = parentPatch.PatchNumber
	}

	patchVersion := &Version{
		Id:                  p.Id.Hex(),
		CreateTime:          time.Now(),
		Identifier:          p.Project,
		Revision:            p.Githash,
		Author:              p.Author,
		Message:             p.Description,
		BuildIds:            []string{},
		BuildVariants:       []VersionBuildStatus{},
		Status:              evergreen.PatchCreated,
		Requester:           requester,
		ParentPatchID:       p.Triggers.ParentPatch,
		ParentPatchNumber:   parentPatchNumber,
		Branch:              projectRef.Branch,
		RevisionOrderNumber: p.PatchNumber,
		AuthorID:            p.Author,
		Parameters:          p.Parameters,
		Activated:           utility.TruePtr(),
	}
	intermediateProject.CreateTime = patchVersion.CreateTime

	tasks := TaskVariantPairs{}
	if len(p.VariantsTasks) > 0 {
		tasks = VariantTasksToTVPairs(p.VariantsTasks)
	} else {
		// handle case where the patch is being finalized but only has the old schema tasks/variants
		// instead of the new one.
		for _, v := range p.BuildVariants {
			for _, t := range p.Tasks {
				if project.FindTaskForVariant(t, v) != nil {
					tasks.ExecTasks = append(tasks.ExecTasks, TVPair{v, t})
				}
			}
		}
		p.VariantsTasks = (&TaskVariantPairs{
			ExecTasks:    tasks.ExecTasks,
			DisplayTasks: tasks.DisplayTasks,
		}).TVPairsToVariantTasks()
	}

	// if variant tasks is still empty, then the patch is empty and we shouldn't add to commit queue
	if p.IsCommitQueuePatch() && len(p.VariantsTasks) == 0 {
		return nil, errors.Errorf("No builds or tasks for commit queue version in projects '%s', githash '%s'", p.Project, p.Githash)
	}
	taskIds := NewPatchTaskIdTable(project, patchVersion, tasks, projectRef.Identifier)
	variantsProcessed := map[string]bool{}

	createTime, err := getTaskCreateTime(p.Project, patchVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get create time for tasks in '%s', githash '%s'", p.Project, p.Githash)
	}

	buildsToInsert := build.Builds{}
	tasksToInsert := task.Tasks{}
	for _, vt := range p.VariantsTasks {
		if _, ok := variantsProcessed[vt.Variant]; ok {
			continue
		}
		variantsProcessed[vt.Variant] = true

		var displayNames []string
		for _, dt := range vt.DisplayTasks {
			displayNames = append(displayNames, dt.Name)
		}
		taskNames := tasks.ExecTasks.TaskNames(vt.Variant)
		buildArgs := BuildCreateArgs{
			Project:           *project,
			Version:           *patchVersion,
			TaskIDs:           taskIds,
			BuildName:         vt.Variant,
			ActivateBuild:     true,
			TaskNames:         taskNames,
			DisplayNames:      displayNames,
			DistroAliases:     distroAliases,
			TaskCreateTime:    createTime,
			SyncAtEndOpts:     p.SyncAtEndOpts,
			ProjectIdentifier: projectRef.Identifier,
		}
		var build *build.Build
		var tasks task.Tasks
		build, tasks, err = CreateBuildFromVersionNoInsert(buildArgs)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if len(tasks) == 0 {
			grip.Info(message.Fields{
				"op":      "skipping empty build for patch version",
				"variant": vt.Variant,
				"version": patchVersion.Id,
			})
			continue
		}

		buildsToInsert = append(buildsToInsert, build)
		tasksToInsert = append(tasksToInsert, tasks...)

		patchVersion.BuildIds = append(patchVersion.BuildIds, build.Id)
		patchVersion.BuildVariants = append(patchVersion.BuildVariants,
			VersionBuildStatus{
				BuildVariant:     vt.Variant,
				BuildId:          build.Id,
				ActivationStatus: ActivationStatus{Activated: true},
			},
		)
	}
	mongoClient := evergreen.GetEnvironment().Client()
	session, err := mongoClient.StartSession()
	if err != nil {
		return nil, errors.Wrap(err, "unable to start session")
	}
	defer session.EndSession(ctx)

	txFunc := func(sessCtx mongo.SessionContext) (interface{}, error) {
		db := evergreen.GetEnvironment().DB()
		_, err = db.Collection(VersionCollection).InsertOne(sessCtx, patchVersion)
		if err != nil {
			return nil, errors.Wrapf(err, "error inserting version '%s'", patchVersion.Id)
		}
		_, err = db.Collection(ParserProjectCollection).InsertOne(sessCtx, intermediateProject)
		if err != nil {
			return nil, errors.Wrapf(err, "error inserting parser project for version '%s'", patchVersion.Id)
		}
		if err = buildsToInsert.InsertMany(sessCtx, false); err != nil {
			return nil, errors.Wrapf(err, "error inserting builds for version '%s'", patchVersion.Id)
		}
		if err = tasksToInsert.InsertUnordered(sessCtx); err != nil {
			return nil, errors.Wrapf(err, "error inserting tasks for version '%s'", patchVersion.Id)
		}
		if err = p.SetActivated(sessCtx, patchVersion.Id); err != nil {
			return nil, errors.Wrapf(err, "error activating patch '%s'", patchVersion.Id)
		}
		return nil, err
	}

	_, err = session.WithTransaction(ctx, txFunc)
	if err != nil {
		return nil, errors.Wrap(err, "unable to finalize patch")
	}

	if p.IsParent() {
		//finalize child patches or subscribe on parent outcome based on parentStatus
		for _, childPatch := range p.Triggers.ChildPatches {
			err = finalizeOrSubscribeChildPatch(ctx, childPatch, p, requester, githubOauthToken)
			if err != nil {
				return nil, errors.Wrap(err, "unable to finalize child patch")
			}
		}
	}
	if len(tasksToInsert) > evergreen.NumTasksForLargePatch {
		numTasksActivated := 0
		for _, t := range tasksToInsert {
			if t.Activated {
				numTasksActivated++
			}
		}
		grip.Info(message.Fields{
			"message":             "version has large number of activated tasks",
			"op":                  "finalize patch",
			"num_tasks_activated": numTasksActivated,
			"total_tasks":         len(tasksToInsert),
			"version":             patchVersion.Id,
		})
	}

	return patchVersion, nil
}

func getLoadProjectOptsForPatch(p *patch.Patch, githubOauthToken string) (*ProjectRef, *GetProjectOpts, error) {
	projectRef, err := FindMergedProjectRef(p.Project, p.Version, true)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if projectRef == nil {
		return nil, nil, errors.Errorf("project '%s' doesn't exist", p.Project)
	}
	hash := p.Githash
	if p.IsGithubPRPatch() {
		hash = p.GithubPatchData.HeadHash
	}
	if p.IsPRMergePatch() {
		hash = p.GithubPatchData.MergeCommitSHA
	}
	opts := GetProjectOpts{
		Ref:          projectRef,
		Token:        githubOauthToken,
		ReadFileFrom: ReadFromPatch,
		Revision:     hash,
		PatchOpts: &PatchOpts{
			patch: p,
		},
	}
	return projectRef, &opts, nil
}

func finalizeOrSubscribeChildPatch(ctx context.Context, childPatchId string, parentPatch *patch.Patch, requester string, githubOauthToken string) error {
	intent, err := patch.FindIntent(childPatchId, patch.TriggerIntentType)
	if err != nil {
		return errors.Errorf("error fetching child patch intent: %s", err)
	}
	if intent == nil {
		return errors.New("childpatch intent not found")
	}
	triggerIntent, ok := intent.(*patch.TriggerIntent)
	if !ok {
		return errors.Errorf("intent '%s' didn't not have expected type '%T'", intent.ID(), intent)
	}
	// if the parentStatus is "", finalize without waiting for the parent patch to finish running
	if triggerIntent.ParentStatus == "" {
		childPatchDoc, err := patch.FindOneId(childPatchId)
		if err != nil {
			return errors.Errorf("error fetching child patch: %s", err)
		}
		if _, err := FinalizePatch(ctx, childPatchDoc, requester, githubOauthToken); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "Failed to finalize child patch document",
				"source":        requester,
				"patch_id":      childPatchId,
				"variants":      childPatchDoc.BuildVariants,
				"tasks":         childPatchDoc.Tasks,
				"variant_tasks": childPatchDoc.VariantsTasks,
				"alias":         childPatchDoc.Alias,
				"parentPatch":   childPatchDoc.Triggers.ParentPatch,
			}))
			return err
		}
	} else {
		//subscribe on parent outcome
		if err = SubscribeOnParentOutcome(triggerIntent.ParentStatus, childPatchId, parentPatch, requester); err != nil {
			return errors.Wrap(err, "problem getting parameters from parent patch")
		}
	}
	return nil
}

func SubscribeOnParentOutcome(parentStatus string, childPatchId string, parentPatch *patch.Patch, requester string) error {
	subscriber := event.NewRunChildPatchSubscriber(event.ChildPatchSubscriber{
		ParentStatus: parentStatus,
		ChildPatchId: childPatchId,
		Requester:    requester,
	})
	patchSub := event.NewParentPatchSubscription(parentPatch.Id.Hex(), subscriber)
	if err := patchSub.Upsert(); err != nil {
		return errors.Wrapf(err, "failed to insert child patch subscription '%s'", childPatchId)
	}
	return nil
}

func CancelPatch(p *patch.Patch, reason task.AbortInfo) error {
	if p.Version != "" {
		if err := SetVersionActivation(p.Version, false, reason.User); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(task.AbortVersion(p.Version, reason))
	}

	return errors.WithStack(patch.Remove(patch.ById(p.Id)))
}

// AbortPatchesWithGithubPatchData runs CancelPatch on patches created before
// the given time, with the same pr number, and base repository. Tasks which
// are abortable (see model/task.IsAbortable()) will be aborted, while
// dispatched/running/completed tasks will not be affected
func AbortPatchesWithGithubPatchData(createdBefore time.Time, closed bool, newPatch, owner, repo string, prNumber int) error {
	patches, err := patch.Find(patch.ByGithubPRAndCreatedBefore(createdBefore, owner, repo, prNumber))
	if err != nil {
		return errors.Wrap(err, "initial patch fetch failed")
	}
	grip.Info(message.Fields{
		"source":         "github hook",
		"created_before": createdBefore.String(),
		"owner":          owner,
		"repo":           repo,
		"message":        "fetched patches to abort",
		"num_patches":    len(patches),
	})

	catcher := grip.NewSimpleCatcher()
	for _, p := range patches {
		if p.Version != "" {
			if p.IsCommitQueuePatch() {
				mergeTask, err := task.FindMergeTaskForVersion(p.Version)
				if err != nil {
					return errors.Wrap(err, "unable to find merge task for version")
				}
				if mergeTask == nil {
					return errors.New("no merge task found")
				}
				catcher.Add(DequeueAndRestart(mergeTask, evergreen.APIServerTaskActivator, "new push to pull request"))
			} else if err = CancelPatch(&p, task.AbortInfo{User: evergreen.GithubPatchUser, NewVersion: newPatch, PRClosed: closed}); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":         "github hook",
					"created_before": createdBefore.String(),
					"owner":          owner,
					"repo":           repo,
					"message":        "failed to abort patch's version",
					"patch_id":       p.Id,
					"pr":             p.GithubPatchData.PRNumber,
					"project":        p.Project,
					"version":        p.Version,
				}))

				catcher.Add(err)
			}
		}
	}

	return errors.Wrap(catcher.Resolve(), "error aborting patches")
}

func MakeCommitQueueDescription(patches []patch.ModulePatch, projectRef *ProjectRef, project *Project) string {
	commitFmtString := "'%s' into '%s/%s:%s'"
	description := []string{}
	for _, p := range patches {
		// skip empty patches
		if len(p.PatchSet.CommitMessages) == 0 {
			continue
		}
		owner := projectRef.Owner
		repo := projectRef.Repo
		branch := projectRef.Branch
		if p.ModuleName != "" {
			module, err := project.GetModuleByName(p.ModuleName)
			if err != nil {
				continue
			}
			owner, repo = module.GetRepoOwnerAndName()
			branch = module.Branch
		}

		description = append(description, fmt.Sprintf(commitFmtString, strings.Join(p.PatchSet.CommitMessages, " <- "), owner, repo, branch))
	}

	if len(description) == 0 {
		description = []string{"No Commits Added"}
	}

	return "Commit Queue Merge: " + strings.Join(description, " || ")
}

type EnqueuePatch struct {
	PatchID string
}

func (e *EnqueuePatch) String() string {
	return fmt.Sprintf("enqueue patch '%s'", e.PatchID)
}

func (e *EnqueuePatch) Send() error {
	existingPatch, err := patch.FindOneId(e.PatchID)
	if err != nil {
		return errors.Wrap(err, "can't get existing patch")
	}
	if existingPatch == nil {
		return errors.Errorf("no patch '%s' found", e.PatchID)
	}

	// only enqueue once
	if len(existingPatch.MergePatch) != 0 {
		return nil
	}

	cq, err := commitqueue.FindOneId(existingPatch.Project)
	if err != nil {
		return errors.Wrap(err, "can't get commit queue")
	}
	if cq == nil {
		return errors.Errorf("no commit queue for project '%s'", existingPatch.Project)
	}

	ctx := context.Background()
	mergePatch, err := MakeMergePatchFromExisting(ctx, existingPatch, "")
	if err != nil {
		return errors.Wrap(err, "problem making merge patch")
	}

	_, err = cq.Enqueue(commitqueue.CommitQueueItem{Issue: mergePatch.Id.Hex(), PatchId: mergePatch.Id.Hex(), Source: commitqueue.SourceDiff})

	return errors.Wrap(err, "can't enqueue item")
}

func (e *EnqueuePatch) Valid() bool {
	return patch.IsValidId(e.PatchID)
}

func MakeMergePatchFromExisting(ctx context.Context, existingPatch *patch.Patch, commitMessage string) (*patch.Patch, error) {
	if !existingPatch.HasValidGitInfo() {
		return nil, errors.Errorf("can't enqueue patch '%s' without metadata", existingPatch.Id.Hex())
	}

	// verify the commit queue is on
	projectRef, err := FindMergedProjectRef(existingPatch.Project, existingPatch.Version, true)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get project ref '%s'", existingPatch.Project)
	}
	if projectRef == nil {
		return nil, errors.Errorf("project ref '%s' doesn't exist", existingPatch.Project)
	}
	if err = projectRef.CommitQueueIsOn(); err != nil {
		return nil, errors.WithStack(err)
	}

	project := &Project{}
	if _, err = LoadProjectInto(ctx, []byte(existingPatch.PatchedConfig), nil, existingPatch.Project, project); err != nil {
		return nil, errors.Wrap(err, "problem loading project")
	}

	patchDoc := &patch.Patch{
		Id:            mgobson.NewObjectId(),
		Author:        existingPatch.Author,
		Project:       existingPatch.Project,
		Githash:       existingPatch.Githash,
		Status:        evergreen.PatchCreated,
		Alias:         evergreen.CommitQueueAlias,
		PatchedConfig: existingPatch.PatchedConfig,
		CreateTime:    time.Now(),
	}

	if patchDoc.Patches, err = patch.MakeMergePatchPatches(existingPatch, commitMessage); err != nil {
		return nil, errors.Wrap(err, "can't make merge patches from existing patch")
	}
	patchDoc.Description = MakeCommitQueueDescription(patchDoc.Patches, projectRef, project)

	// verify the commit queue has tasks/variants enabled that match the project
	project.BuildProjectTVPairs(patchDoc, patchDoc.Alias)
	if len(patchDoc.Tasks) == 0 && len(patchDoc.BuildVariants) == 0 {
		return nil, errors.New("commit queue has no build variants or tasks configured")
	}

	u, err := user.FindOneById(patchDoc.Author)
	if err != nil {
		return nil, errors.Wrapf(err, "can't find user for patch author '%s'", patchDoc.Author)
	}
	if u == nil {
		return nil, errors.Errorf("patch author '%s' not found", patchDoc.Author)
	}
	// get the next patch number for the user
	patchDoc.PatchNumber, err = u.IncPatchNumber()
	if err != nil {
		return nil, errors.Wrap(err, "error computing patch num")
	}

	if err = patchDoc.Insert(); err != nil {
		return nil, errors.Wrap(err, "can't insert patch")
	}

	if err = existingPatch.SetMergePatch(patchDoc.Id.Hex()); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":        "can't mark patch enqueued",
			"existing_patch": existingPatch.Id.Hex(),
			"merge_patch":    patchDoc.Id.Hex(),
		}))
	}

	return patchDoc, nil
}

func RetryCommitQueueItems(projectID string, opts RestartOptions) ([]string, []string, error) {
	patches, err := patch.FindFailedCommitQueuePatchesinTimeRange(projectID, opts.StartTime, opts.EndTime)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error finding failed commit queue patches for project in time range")
	}
	cq, err := commitqueue.FindOneId(projectID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error finding commit queue '%s'", projectID)
	}
	if cq == nil {
		return nil, nil, errors.Errorf("commit queue '%s' not found", projectID)
	}

	// don't requeue items, just return what would be requeued
	if opts.DryRun {
		toBeRequeued := []string{}
		for _, p := range patches {
			toBeRequeued = append(toBeRequeued, p.Id.Hex())
		}
		return toBeRequeued, nil, nil
	}
	patchesRestarted := []string{}
	patchesFailed := []string{}
	for _, p := range patches {
		// use the PR number to determine if this is a PR or diff patch. Currently there
		// is not a reliable field that is set for diff patches
		var err error
		if p.GithubPatchData.PRNumber > 0 {
			err = restartPRItem(p, cq)
		} else {
			err = restartDiffItem(p, cq)
		}
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"patch":        p.Id,
				"commit_queue": cq.ProjectID,
				"message":      "error restarting commit queue item",
				"pr_number":    p.GithubPatchData.PRNumber,
			}))
			patchesFailed = append(patchesFailed, p.Id.Hex())
		} else {
			patchesRestarted = append(patchesRestarted, p.Id.Hex())
		}
	}

	return patchesRestarted, patchesFailed, nil
}

func restartPRItem(p patch.Patch, cq *commitqueue.CommitQueue) error {
	// reconstruct commit queue item from patch
	modules := []commitqueue.Module{}
	for _, modulePatch := range p.Patches {
		if modulePatch.ModuleName != "" {
			module := commitqueue.Module{
				Module: modulePatch.ModuleName,
				Issue:  modulePatch.PatchSet.Patch,
			}
			modules = append(modules, module)
		}
	}
	item := commitqueue.CommitQueueItem{
		Issue:   strconv.Itoa(p.GithubPatchData.PRNumber),
		Modules: modules,
		Source:  commitqueue.SourcePullRequest,
	}
	if _, err := cq.Enqueue(item); err != nil {
		return errors.Wrap(err, "error enqueuing item")
	}

	return nil
}

func restartDiffItem(p patch.Patch, cq *commitqueue.CommitQueue) error {
	u, err := user.FindOne(user.ById(p.Author))
	if err != nil {
		return errors.Wrapf(err, "error finding user %s", p.Author)
	}
	if u == nil {
		return errors.Errorf("user %s not found", p.Author)
	}
	patchNumber, err := u.IncPatchNumber()
	if err != nil {
		return errors.Wrap(err, "error incrementing patch number")
	}
	newPatch := patch.Patch{
		Id:              mgobson.NewObjectId(),
		Project:         p.Project,
		Author:          p.Author,
		Githash:         p.Githash,
		CreateTime:      time.Now(),
		Status:          evergreen.PatchCreated,
		Description:     p.Description,
		GithubPatchData: p.GithubPatchData,
		Tasks:           p.Tasks,
		VariantsTasks:   p.VariantsTasks,
		BuildVariants:   p.BuildVariants,
		Alias:           p.Alias,
		Patches:         p.Patches,
		PatchNumber:     patchNumber,
	}

	if err = newPatch.Insert(); err != nil {
		return errors.Wrap(err, "error inserting patch")
	}
	if _, err = cq.Enqueue(commitqueue.CommitQueueItem{Issue: newPatch.Id.Hex(), PatchId: newPatch.Id.Hex(), Source: commitqueue.SourceDiff}); err != nil {
		return errors.Wrap(err, "error enqueuing item")
	}
	return nil
}

func SendCommitQueueResult(p *patch.Patch, status message.GithubState, description string) error {
	if p.GithubPatchData.PRNumber == 0 {
		return nil
	}
	projectRef, err := FindMergedProjectRef(p.Project, p.Version, true)
	if err != nil {
		return errors.Wrap(err, "unable to find project")
	}
	if projectRef == nil {
		return errors.New("no project found for patch")
	}
	url := ""
	if p.Version != "" {
		settings, err := evergreen.GetConfig()
		if err != nil {
			return errors.Wrap(err, "unable to get settings")
		}
		url = fmt.Sprintf("%s/version/%s", settings.Ui.Url, p.Version)
	}
	msg := message.GithubStatus{
		Owner:       projectRef.Owner,
		Repo:        projectRef.Repo,
		Ref:         p.GithubPatchData.HeadHash,
		Context:     commitqueue.GithubContext,
		State:       status,
		Description: description,
		URL:         url,
	}
	sender, err := evergreen.GetEnvironment().GetSender(evergreen.SenderGithubStatus)
	if err != nil {
		return errors.Wrap(err, "unable to get github sender")
	}
	sender.Send(message.NewGithubStatusMessageWithRepo(level.Notice, msg))

	return nil
}
