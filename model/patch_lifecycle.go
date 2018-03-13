package model

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
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
func AddNewBuildsForPatch(p *patch.Patch, patchVersion *version.Version, project *Project, tasks TaskVariantPairs) error {
	return AddNewBuilds(p.Activated, patchVersion, project, tasks)
}

// Given a patch version and set of variant/task pairs, creates any tasks that don't exist yet,
// within the set of already existing builds.
func AddNewTasksForPatch(p *patch.Patch, patchVersion *version.Version, project *Project, pairs TaskVariantPairs) error {
	return AddNewTasks(p.Activated, patchVersion, project, pairs)
}

// IncludePatchDependencies takes a project and a slice of variant/task pairs names
// and returns the expanded set of variant/task pairs to include all the dependencies/requirements
// for the given set of tasks.
// If any dependency is cross-variant, it will include the variant and task for that dependency.
func IncludePatchDependencies(project *Project, tvpairs []TVPair) []TVPair {
	di := &dependencyIncluder{Project: project}
	return di.Include(tvpairs)
}

// MakePatchedConfig takes in the path to a remote configuration a stringified version
// of the current project and returns an unmarshalled version of the project
// with the patch applied
func MakePatchedConfig(p *patch.Patch, remoteConfigPath, projectConfig string) (
	*Project, error) {
	for _, patchPart := range p.Patches {
		// we only need to patch the main project and not any other modules
		if patchPart.ModuleName != "" {
			continue
		}

		var patchFilePath string
		var err error
		if patchPart.PatchSet.Patch == "" {
			reader, err := db.GetGridFile(patch.GridFSPrefix, patchPart.PatchSet.PatchFileId)
			if err != nil {
				return nil, errors.Wrap(err, "Can't fetch patch file from gridfs")
			}
			defer reader.Close()
			bytes, err := ioutil.ReadAll(reader)
			if err != nil {
				return nil, errors.Wrap(err, "Can't read patch file contents from gridfs")
			}

			patchFilePath, err = util.WriteToTempFile(string(bytes))
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

		stderr := send.MakeWriterSender(grip.GetSender(), level.Error)
		defer stderr.Close() //nolint: evg
		stdout := send.MakeWriterSender(grip.GetSender(), level.Info)
		defer stdout.Close() //nolint: evg
		output := subprocess.OutputOptions{Output: stdout, Error: stderr}

		patchCmd := subprocess.NewLocalCommand(
			strings.Join(patchCommandStrings, "\n"),
			workingDirectory,
			"bash",
			nil,
			true)

		if err = patchCmd.SetOutput(output); err != nil {
			return nil, errors.Wrap(err, "problem configuring command output")
		}

		ctx := context.TODO()
		if err = patchCmd.Run(ctx); err != nil {
			return nil, errors.Errorf("could not run patch command: %v", err)
		}
		// read in the patched config file
		data, err := ioutil.ReadFile(localConfigPath)
		if err != nil {
			return nil, errors.Wrap(err, "could not read patched config file")
		}
		project := &Project{}
		if err = LoadProjectInto(data, p.Project, project); err != nil {
			return nil, errors.WithStack(err)
		}
		return project, nil
	}
	return nil, errors.New("no patch on project")
}

// Finalizes a patch:
// Patches a remote project's configuration file if needed.
// Creates a version for this patch and links it.
// Creates builds based on the version.
func FinalizePatch(p *patch.Patch, requester string, githubOauthToken string) (*version.Version, error) {
	// unmarshal the project YAML for storage
	project := &Project{}
	err := LoadProjectInto([]byte(p.PatchedConfig), p.Project, project)
	if err != nil {
		return nil, errors.Wrapf(err,
			"Error marshaling patched project config from repository revision “%v”",
			p.Githash)
	}

	projectRef, err := FindOneProjectRef(p.Project)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	gitCommit, err := thirdparty.GetCommitEvent(githubOauthToken, projectRef.Owner, projectRef.Repo, p.Githash)
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't fetch commit information")
	}
	if gitCommit == nil {
		return nil, errors.New("Couldn't fetch commit information; git commit doesn't exist")
	}

	patchVersion := &version.Version{
		Id:                  p.Id.Hex(),
		CreateTime:          time.Now(),
		Identifier:          p.Project,
		Revision:            p.Githash,
		Author:              p.Author,
		Message:             p.Description,
		BuildIds:            []string{},
		BuildVariants:       []version.BuildStatus{},
		Config:              p.PatchedConfig,
		Status:              evergreen.PatchCreated,
		Requester:           requester,
		Branch:              projectRef.Branch,
		RevisionOrderNumber: p.PatchNumber,
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

		var buildId string
		var displayNames []string
		for _, dt := range vt.DisplayTasks {
			displayNames = append(displayNames, dt.Name)
		}
		taskNames := tasks.ExecTasks.TaskNames(vt.Variant)
		buildId, err = CreateBuildFromVersion(project, patchVersion, taskIds, vt.Variant, true, taskNames, displayNames)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		patchVersion.BuildIds = append(patchVersion.BuildIds, buildId)
		patchVersion.BuildVariants = append(patchVersion.BuildVariants,
			version.BuildStatus{
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
		return errors.WithStack(AbortVersion(p.Version))
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
