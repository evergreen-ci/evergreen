package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
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
// for the same variant together under a single list, and return all the variant groups
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
func AddNewBuildsForPatch(p *patch.Patch, patchVersion *Version, project *Project, tasks TaskVariantPairs) error {
	return AddNewBuilds(p.Activated, patchVersion, project, tasks, "")
}

// Given a patch version and set of variant/task pairs, creates any tasks that don't exist yet,
// within the set of already existing builds.
func AddNewTasksForPatch(ctx context.Context, p *patch.Patch, patchVersion *Version, project *Project, pairs TaskVariantPairs) error {
	return AddNewTasks(ctx, p.Activated, patchVersion, project, pairs, "")
}

// IncludePatchDependencies takes a project and a slice of variant/task pairs names
// and returns the expanded set of variant/task pairs to include all the dependencies/requirements
// for the given set of tasks.
// If any dependency is cross-variant, it will include the variant and task for that dependency.
func IncludePatchDependencies(project *Project, tvpairs []TVPair) []TVPair {
	di := &dependencyIncluder{Project: project}
	return di.Include(tvpairs)
}

// GetPatchedProject creates and validates a project created by fetching latest commit information from GitHub
// and applying the patch to the latest remote configuration. Also returns the condensed yaml string for storage.
// The error returned can be a validation error.
func GetPatchedProject(ctx context.Context, p *patch.Patch, githubOauthToken string) (*Project, string, error) {
	if p.Version != "" {
		return nil, "", errors.Errorf("Patch %v already finalized", p.Version)
	}

	projectRef, err := FindOneProjectRef(p.Project)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}

	project := &Project{}
	// if the patched config exists, use that as the project file bytes.
	if p.PatchedConfig != "" {
		if err = LoadProjectInto([]byte(p.PatchedConfig), projectRef.Identifier, project); err != nil {
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

	githubFile, err := thirdparty.GetGithubFile(ctx, githubOauthToken, projectRef.Owner,
		projectRef.Repo, projectRef.RemotePath, hash)
	if err != nil {
		// if the project file doesn't exist, but our patch includes a project file,
		// we try to apply the diff and proceed.
		if !(p.ConfigChanged(projectRef.RemotePath) && thirdparty.IsFileNotFound(err)) {
			// return an error if the github error is network/auth-related or we aren't patching the config
			return nil, "", errors.Wrapf(err, "Could not get github file at '%s/%s'@%s: %s", projectRef.Owner,
				projectRef.Repo, projectRef.RemotePath, hash)
		}
	} else {
		// we successfully got the project file in base64, so we decode it
		projectFileBytes, err = base64.StdEncoding.DecodeString(*githubFile.Content)
		if err != nil {
			return nil, "", errors.Wrapf(err, "Could not decode github file at '%s/%s'@%s: %s", projectRef.Owner,
				projectRef.Repo, projectRef.RemotePath, hash)
		}
	}

	// apply remote configuration patch if needed
	if !(p.IsGithubPRPatch() || p.IsPRMergePatch()) && p.ConfigChanged(projectRef.RemotePath) {
		projectFileBytes, err = MakePatchedConfig(ctx, env, p, projectRef.RemotePath, string(projectFileBytes))
		if err != nil {
			return nil, "", errors.Wrapf(err, "Could not patch remote configuration file")
		}
	}

	if err := LoadProjectInto(projectFileBytes, projectRef.Identifier, project); err != nil {
		return nil, "", errors.WithStack(err)
	}

	return project, string(projectFileBytes), nil
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
			var reader io.ReadCloser
			reader, err = db.GetGridFile(patch.GridFSPrefix, patchPart.PatchSet.PatchFileId)
			if err != nil {
				return nil, errors.Wrap(err, "Can't fetch patch file from gridfs")
			}
			defer reader.Close()

			var data []byte
			data, err = ioutil.ReadAll(reader)
			if err != nil {
				return nil, errors.Wrap(err, "Can't read patch file contents from gridfs")
			}

			patchFilePath, err = util.WriteToTempFile(string(data))
			if err != nil {
				return nil, errors.Wrap(err, "could not write temporary patch file")
			}
		} else {
			patchFilePath, err = util.WriteToTempFile(patchPart.PatchSet.Patch)
			if err != nil {
				return nil, errors.Wrap(err, "could not write temporary patch file")
			}
		}

		defer os.Remove(patchFilePath) //nolint: evg
		// write project configuration
		configFilePath, err := util.WriteToTempFile(projectConfig)
		if err != nil {
			return nil, errors.Wrap(err, "could not write config file")
		}
		defer os.Remove(configFilePath) //nolint: evg

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
			fmt.Sprintf("set -o xtrace"),
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("git apply --whitespace=fix --include=%v < '%v'",
				remoteConfigPath, patchFilePath),
		}

		err = env.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", strings.Join(patchCommandStrings, "\n")}).
			SetErrorSender(level.Error, grip.GetSender()).SetOutputSender(level.Info, grip.GetSender()).
			Directory(workingDirectory).Run(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not run patch command")
		}

		// read in the patched config file
		data, err := ioutil.ReadFile(localConfigPath)
		if err != nil {
			return nil, errors.Wrap(err, "could not read patched config file")
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
	err := LoadProjectInto([]byte(p.PatchedConfig), p.Project, project)
	if err != nil {
		return nil, errors.Wrapf(err,
			"Error marshaling patched project config from repository revision “%v”",
			p.Githash)
	}

	distroAliases, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return nil, errors.Wrap(err, "problem resolving distro alias table for patch")
	}

	projectRef, err := FindOneProjectRef(p.Project)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = thirdparty.GetCommitEvent(ctx, githubOauthToken, projectRef.Owner, projectRef.Repo, p.Githash)
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't fetch commit information")
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
		Config:              p.PatchedConfig,
		Status:              evergreen.PatchCreated,
		Requester:           requester,
		Branch:              projectRef.Branch,
		RevisionOrderNumber: p.PatchNumber,
		AuthorID:            p.Author,
	}

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

	taskIds := NewPatchTaskIdTable(project, patchVersion, tasks)
	variantsProcessed := map[string]bool{}
	for _, vt := range p.VariantsTasks {
		if _, ok := variantsProcessed[vt.Variant]; ok {
			continue
		}
		variantsProcessed[vt.Variant] = true

		var buildId string
		var displayNames []string
		for _, dt := range vt.DisplayTasks {
			displayNames = append(displayNames, dt.Name)
		}
		taskNames := tasks.ExecTasks.TaskNames(vt.Variant)
		buildArgs := BuildCreateArgs{
			Project:       *project,
			Version:       *patchVersion,
			TaskIDs:       taskIds,
			BuildName:     vt.Variant,
			Activated:     true,
			TaskNames:     taskNames,
			DisplayNames:  displayNames,
			DistroAliases: distroAliases,
		}
		buildId, err = CreateBuildFromVersion(buildArgs)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		patchVersion.BuildIds = append(patchVersion.BuildIds, buildId)
		patchVersion.BuildVariants = append(patchVersion.BuildVariants,
			VersionBuildStatus{
				BuildVariant: vt.Variant,
				Activated:    true,
				BuildId:      buildId,
			},
		)
	}

	if err = patchVersion.Insert(); err != nil {
		return nil, errors.WithStack(err)
	}

	if err = p.SetActivated(patchVersion.Id); err != nil {
		return nil, errors.WithStack(err)
	}
	return patchVersion, nil
}

func CancelPatch(p *patch.Patch, caller string) error {
	if p.Version != "" {
		if err := SetVersionActivation(p.Version, false, caller); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(AbortVersion(p.Version, caller))
	}

	return errors.WithStack(patch.Remove(patch.ById(p.Id)))
}

// AbortPatchesWithGithubPatchData runs CancelPatch on patches created before
// the given time, with the same pr number, and base repository. Tasks which
// are abortable (see model/task.IsAbortable()) will be aborted, while
// dispatched/running/completed tasks will not be affected
func AbortPatchesWithGithubPatchData(createdBefore time.Time, owner, repo string, prNumber int) error {
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
	for i, _ := range patches {
		if patches[i].Version != "" {
			if err = CancelPatch(&patches[i], evergreen.GithubPRRequester); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":         "github hook",
					"created_before": createdBefore.String(),
					"owner":          owner,
					"repo":           repo,
					"message":        "failed to abort patch's version",
					"patch_id":       patches[i].Id,
					"version":        patches[i].Version,
				}))

				catcher.Add(err)
			}
		}
	}

	return errors.Wrap(catcher.Resolve(), "error aborting patches")
}

func RetryCommitQueueItems(projectID string, patchType string, opts RestartOptions) ([]string, []string, error) {
	patches, err := patch.FindFailedCommitQueuePatchesinTimeRange(projectID, opts.StartTime, opts.EndTime)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error finding failed commit queue patches for project in time range")
	}
	cq, err := commitqueue.FindOneId(projectID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error finding commit queue '%s'", projectID)
	}
	if cq == nil {
		return nil, nil, errors.New("commit queue '%s' not found")
	}

	// don't requeue items, just return what would be requeued
	if opts.DryRun {
		toBeRequeued := []string{}
		for _, p := range patches {
			toBeRequeued = append(toBeRequeued, p.Id.Hex())
		}
		return toBeRequeued, nil, nil
	}
	if patchType == commitqueue.PRPatchType {
		restarted, notRestarted := restartPRItems(patches, cq)
		return restarted, notRestarted, nil
	}
	if patchType == commitqueue.CLIPatchType {
		restarted, notRestarted := restartCLIItems(patches, cq)
		return restarted, notRestarted, nil
	}
	return nil, nil, errors.Errorf("patch type '%s' is invalid", patchType)
}

func restartPRItems(patches []patch.Patch, cq *commitqueue.CommitQueue) ([]string, []string) {
	restartedPatches := []string{}
	patchesWithErrors := []string{}
	for _, p := range patches {
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
		}
		if _, err := cq.Enqueue(item); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"patch":             p.Id,
				"commit_queue":      cq.ProjectID,
				"commit_queue_type": commitqueue.PRPatchType,
				"item":              item.Issue,
				"message":           "error enqueuing item",
				"operation":         "restart failed commit queue versions",
			}))
			patchesWithErrors = append(patchesWithErrors, p.Id.Hex())
			continue
		}
		restartedPatches = append(restartedPatches, p.Id.Hex())
	}
	return restartedPatches, patchesWithErrors
}

func restartCLIItems(patches []patch.Patch, cq *commitqueue.CommitQueue) ([]string, []string) {
	restartedPatches := []string{}
	patchesWithErrors := []string{}
	for _, p := range patches {
		u, err := user.FindOne(user.ById(p.Author))
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"patch":             p.Id,
				"commit_queue":      cq.ProjectID,
				"commit_queue_type": commitqueue.CLIPatchType,
				"user":              p.Author,
				"message":           "error finding user patch",
				"operation":         "restart failed commit queue versions",
			}))
			patchesWithErrors = append(patchesWithErrors, p.Id.Hex())
			continue
		}
		if u == nil {
			grip.Error(message.WrapError(err, message.Fields{
				"patch":             p.Id,
				"commit_queue":      cq.ProjectID,
				"commit_queue_type": commitqueue.CLIPatchType,
				"user":              p.Author,
				"message":           "user for patch not found",
				"operation":         "restart failed commit queue versions",
			}))
			patchesWithErrors = append(patchesWithErrors, p.Id.Hex())
			continue
		}
		patchNumber, err := u.IncPatchNumber()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"patch":             p.Id,
				"commit_queue":      cq.ProjectID,
				"commit_queue_type": commitqueue.CLIPatchType,
				"user":              p.Author,
				"message":           "error computing patch number",
				"operation":         "restart failed commit queue versions",
			}))
			patchesWithErrors = append(patchesWithErrors, p.Id.Hex())
			continue
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
			grip.Error(message.WrapError(err, message.Fields{
				"patch":             p.Id,
				"commit_queue":      cq.ProjectID,
				"commit_queue_type": commitqueue.CLIPatchType,
				"message":           "error inserting new patch",
				"operation":         "restart failed commit queue versions",
			}))
			patchesWithErrors = append(patchesWithErrors, p.Id.Hex())
			continue
		}
		if _, err = cq.Enqueue(commitqueue.CommitQueueItem{Issue: newPatch.Id.Hex()}); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"patch":             p.Id,
				"commit_queue":      cq.ProjectID,
				"commit_queue_type": commitqueue.CLIPatchType,
				"message":           "error enqueueing item",
				"operation":         "restart failed commit queue versions",
			}))
			patchesWithErrors = append(patchesWithErrors, p.Id.Hex())
			continue
		}
		restartedPatches = append(restartedPatches, p.Id.Hex())
	}
	return restartedPatches, patchesWithErrors
}
