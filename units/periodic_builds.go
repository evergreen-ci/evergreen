package units

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const periodicBuildJobName = "periodic-build"

func init() {
	registry.AddJobType(periodicBuildJobName, func() amboy.Job {
		return makePeriodicBuildsJob()
	})
}

type periodicBuildJob struct {
	ProjectID    string `bson:"project_id"`
	DefinitionID string `bson:"def_id"`

	project *model.ProjectRef
	env     evergreen.Environment
	job.Base
}

func makePeriodicBuildsJob() *periodicBuildJob {
	j := &periodicBuildJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    periodicBuildJobName,
				Version: 0,
			},
		},
	}
	return j
}

func NewPeriodicBuildJob(projectID, definitionID string) amboy.Job {
	j := makePeriodicBuildsJob()
	j.ProjectID = projectID
	j.DefinitionID = definitionID
	ts := utility.RoundPartOfHour(15)
	j.SetID(fmt.Sprintf("%s-%s-%s-%s", periodicBuildJobName, projectID, definitionID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s.%s", periodicBuildJobName, projectID, definitionID)})
	j.SetEnqueueAllScopes(true)
	j.UpdateTimeInfo(amboy.JobTimeInfo{WaitUntil: ts})

	return j
}

func (j *periodicBuildJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	var err error
	j.project, err = model.FindMergedProjectRef(j.ProjectID, "", true)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding project '%s'", j.ProjectID))
		return
	}
	var definition *model.PeriodicBuildDefinition
	for _, d := range j.project.PeriodicBuilds {
		if d.ID == j.DefinitionID {
			definition = &d
			break
		}
	}
	if definition == nil {
		j.AddError(errors.Errorf("periodic build definition '%s' not found", j.DefinitionID))
		return
	}
	defer func() {
		baseTime := definition.NextRunTime
		if utility.IsZeroTime(baseTime) {
			baseTime = time.Now()
		}
		err = j.project.UpdateNextPeriodicBuild(definition.ID, baseTime.Add(time.Duration(definition.IntervalHours)*time.Hour))
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "unable to set next periodic build job time",
			"project":    j.ProjectID,
			"definition": j.DefinitionID,
		}))
	}()
	versionID, versionError := j.addVersion(ctx, *definition)

	if versionError != nil {
		// if the version fails to be added, create a stub version and
		// log an event so users can get notified when notifications are configured
		metadata := model.VersionMetadata{
			IsAdHoc:         true,
			Message:         definition.Message,
			PeriodicBuildID: definition.ID,
			Alias:           definition.Alias,
		}
		stubVersion, dbErr := repotracker.ShellVersionFromRevision(ctx, j.project, metadata)
		if dbErr != nil {
			grip.Error(message.WrapError(dbErr, message.Fields{
				"message":            "error creating stub version for periodic build",
				"runner":             periodicBuildJobName,
				"project":            j.project,
				"project_identifier": j.project.Identifier,
				"definitionID":       j.DefinitionID,
			}))
		}
		if stubVersion == nil {
			j.AddError(versionError)
			return
		}
		stubVersion.Errors = []string{versionError.Error()}
		insertError := stubVersion.Insert()
		if err != nil {
			grip.Error(message.WrapError(insertError, message.Fields{
				"message":            "error inserting stub version for periodic build",
				"runner":             periodicBuildJobName,
				"project":            j.project,
				"project_identifier": j.project.Identifier,
				"definitionID":       j.DefinitionID,
			}))
		}
		event.LogVersionStateChangeEvent(stubVersion.Id, evergreen.VersionFailed)

		j.AddError(versionError)
		return
	}

	err = model.SetVersionActivation(versionID, true, evergreen.User)
	if err != nil {
		// if the version fails to activate, log an event so users
		// can get notified when notifications are configured
		event.LogVersionStateChangeEvent(versionID, evergreen.VersionFailed)
		j.AddError(err)
		return
	}

}

func (j *periodicBuildJob) addVersion(ctx context.Context, definition model.PeriodicBuildDefinition) (string, error) {
	token, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		return "", errors.Wrap(err, "getting GitHub OAuth token")
	}

	// kim: NOTE: this uses most recent mainline version as basis for this
	// periodic build. Since it's always based on the previous mainline commit
	// (i.e. periodic build, commit, trigger, git tag, etc), it will only try
	// using a revision from the project's current branch if there is a version
	// that uses the newer branch. Ever since the project settings branch was
	// changed, only periodic builds have run, so they keep using the old
	// branch's revision.
	mostRecentVersion, err := model.VersionFindOne(model.VersionByMostRecentSystemRequester(j.ProjectID))
	if err != nil {
		return "", errors.Wrapf(err, "finding most recent version for project '%s'", j.ProjectID)
	}
	if mostRecentVersion == nil {
		return "", errors.Errorf("no recent version found for project '%s'", j.ProjectID)
	}
	// kim: QUESTION: can we check the most recent version's branch vs. the
	// current project setting's branch? Is there a way to make a version based
	// on a new branch that has no prior versions and no revision to start off
	// with?
	// kim: TODO: look into how repotracker creates versions from commits. Maybe
	// can pull the most recent commit from the correct branch to use as the
	// revision rather than use the most recent version's revision. Not sure how much the task
	// depends on the revision having an associated version, but if it's allowed to be unassociated
	// with a version, then passing just the most recent revision on the new branch might work.
	//
	// Alternatively, it seems like the repotracker never activates the first commit in a project
	// mainline. Maybe because it can't run? Could we fake trigger the repotracker to make one
	// inactive version from the latest branch when it switches and a non-commit version needs a
	// previous revision? That way, it'll set the last revision to one that points to a revision on
	// the "current" branch
	configFile, err := thirdparty.GetGithubFile(ctx, token, j.project.Owner, j.project.Repo, definition.ConfigFile, mostRecentVersion.Revision)
	if err != nil {
		return "", errors.Wrap(err, "getting config file from GitHub")
	}
	configBytes, err := base64.StdEncoding.DecodeString(*configFile.Content)
	if err != nil {
		return "", errors.Wrap(err, "decoding config file")
	}
	proj := &model.Project{}
	opts := &model.GetProjectOpts{
		Ref:          j.project,
		Revision:     mostRecentVersion.Revision,
		Token:        token,
		ReadFileFrom: model.ReadfromGithub,
	}
	intermediateProject, err := model.LoadProjectInto(ctx, configBytes, opts, j.project.Id, proj)
	if err != nil {
		return "", errors.Wrap(err, "parsing config file")
	}
	var config *model.ProjectConfig
	if j.project.IsVersionControlEnabled() {
		config, err = model.CreateProjectConfig(configBytes, j.project.Id)
		if err != nil {
			return "", errors.Wrap(err, "parsing project config")
		}
	}
	metadata := model.VersionMetadata{
		IsAdHoc:         true,
		Message:         definition.Message,
		PeriodicBuildID: definition.ID,
		Alias:           definition.Alias,
		// kim: QUESTION: should this set the branch right here, rather than
		// assume it'll be populated later, when the branch and revision might
		// drift apart?
		Revision: model.Revision{
			Revision: mostRecentVersion.Revision,
		},
	}

	projectInfo := &model.ProjectInfo{
		Ref:                 j.project,
		Project:             proj,
		IntermediateProject: intermediateProject,
		Config:              config,
	}
	v, err := repotracker.CreateVersionFromConfig(ctx, projectInfo, metadata, false, nil)
	if err != nil {
		return "", errors.Wrap(err, "creating version from config")
	}
	if v == nil {
		return "", errors.New("no version created")
	}
	return v.Id, nil
}
