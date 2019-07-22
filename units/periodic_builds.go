package units

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const periodicBuildJobName = "periodic-build"

func init() {
	registry.AddJobType(periodicBuildJobName, func() amboy.Job {
		return makePeriodicBuildsJob()
	})
}

type periodicBuildJob struct {
	ProjectID string `bson:"project_id"`

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

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewPeriodicBuildJob(projectID string, ts string) amboy.Job {
	j := makePeriodicBuildsJob()
	j.ProjectID = projectID
	j.SetID(fmt.Sprintf("%s-%s-%s", periodicBuildJobName, projectID, ts))

	return j
}

func (j *periodicBuildJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	var err error
	j.project, err = model.FindOneProjectRef(j.ProjectID)
	if err != nil {
		j.AddError(errors.Wrap(err, "error finding project"))
		return
	}
	if j.project == nil {
		j.AddError(errors.New("project not found"))
		return
	}

	catcher := grip.NewBasicCatcher()
	for _, definition := range j.project.PeriodicBuilds {
		shouldRerun, err := j.shouldRerun(definition)
		if err != nil {
			catcher.Add(err)
			continue
		}
		if shouldRerun {
			catcher.Add(j.addVersion(ctx, definition))
		}
	}

	j.AddError(catcher.Resolve())
}

func (j *periodicBuildJob) addVersion(ctx context.Context, definition model.PeriodicBuildDefinition) error {
	token, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "error getting github token")
	}
	configFile, err := thirdparty.GetGithubFile(ctx, token, j.project.Owner, j.project.Repo, j.project.RemotePath, "")
	if err != nil {
		return errors.Wrap(err, "error getting config file from github")
	}
	configBytes, err := base64.StdEncoding.DecodeString(*configFile.Content)
	if err != nil {
		return errors.Wrap(err, "error decoding config file")
	}
	config := model.Project{}
	err = model.LoadProjectInto(configBytes, j.project.Identifier, &config)
	if err != nil {
		return errors.Wrap(err, "error parsing config file")
	}
	metadata := repotracker.VersionMetadata{
		IsAdHoc:         true,
		Message:         definition.Message,
		PeriodicBuildID: definition.ID,
	}

	v, err := repotracker.CreateVersionFromConfig(ctx, j.project, &config, metadata, false, nil)
	if err != nil {
		return errors.Wrap(err, "error creating version from config")
	}

	return errors.Wrap(model.ActivateElapsedBuilds(v), "error activating ad hoc build")
}

func (j *periodicBuildJob) shouldRerun(definition model.PeriodicBuildDefinition) (bool, error) {
	lastVersion, err := model.FindLastPeriodicBuild(j.ProjectID, definition.ID)
	if err != nil {
		return false, errors.Wrap(err, "error finding last periodic build")
	}
	if lastVersion == nil {
		return true, nil
	}
	elapsed := time.Now().Sub(lastVersion.CreateTime)
	return elapsed >= time.Duration(definition.IntervalHours)*time.Hour, nil
}
