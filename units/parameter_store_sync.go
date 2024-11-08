package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	parameterStoreSyncJobName = "parameter-store-sync"
)

func init() {
	registry.AddJobType(parameterStoreSyncJobName, func() amboy.Job { return makeParameterStoreSyncJob() })
}

type parameterStoreSyncJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeParameterStoreSyncJob() *parameterStoreSyncJob {
	j := &parameterStoreSyncJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    parameterStoreSyncJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewParameterStoreSyncJob creates a job that syncs project variables to SSM
// Parameter Store for any branch project or repo ref that has Parameter Store
// enabled but whose vars are not already in sync.
// TODO (DEVPROD-11882): remove this job once the rollout is stable.
func NewParameterStoreSyncJob(ts string) amboy.Job {
	j := makeParameterStoreSyncJob()
	j.SetID(fmt.Sprintf("%s.%s", parameterStoreSyncJobName, ts))
	j.SetScopes([]string{parameterStoreSyncJobName})
	j.SetEnqueueAllScopes(true)
	return j
}

func (j *parameterStoreSyncJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags to check if Parameter Store is enabled"))
		return
	}
	if flags.ParameterStoreDisabled {
		return
	}

	pRefs, err := model.FindProjectRefsToSync(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding project refs to sync"))
		return
	}
	j.AddError(errors.Wrap(j.sync(ctx, pRefs, false), "syncing project variables for branch project refs"))

	repoRefs, err := model.FindRepoRefsToSync(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding repo refs to sync"))
		return
	}

	repoProjRefs := make([]model.ProjectRef, 0, len(repoRefs))
	for _, repoRef := range repoRefs {
		repoProjRefs = append(repoProjRefs, repoRef.ProjectRef)
	}
	j.AddError(errors.Wrap(j.sync(ctx, repoProjRefs, true), "syncing project variables for repo refs"))
}

func (j *parameterStoreSyncJob) sync(ctx context.Context, pRefs []model.ProjectRef, areRepoRefs bool) error {
	catcher := grip.NewBasicCatcher()
	for _, pRef := range pRefs {
		pVars, err := model.FindOneProjectVars(pRef.Id)
		if err != nil {
			catcher.Wrapf(err, "finding project vars for project '%s'", pRef.Id)
			continue
		}
		if err := model.FullSyncToParameterStore(ctx, pVars, &pRef, areRepoRefs); err != nil {
			catcher.Wrapf(err, "syncing project vars for project '%s'", pRef.Id)
		}
	}
	return catcher.Resolve()
}
