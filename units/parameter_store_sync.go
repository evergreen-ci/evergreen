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
	"github.com/mongodb/grip/message"
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
		if !pRef.ParameterStoreVarsSynced {
			pVars, err := model.FindOneProjectVars(pRef.Id)
			if err != nil {
				catcher.Wrapf(err, "finding project vars for project '%s'", pRef.Id)
				continue
			}
			if pVars == nil {
				grip.Notice(message.Fields{
					"message":     "found project that has no project vars, initializing with empty project vars",
					"project":     pRef.Id,
					"is_repo_ref": areRepoRefs,
					"job":         j.ID(),
				})
				pVars = &model.ProjectVars{Id: pRef.Id}
			}
			pm, err := model.FullSyncToParameterStore(ctx, pVars, &pRef, areRepoRefs)
			if err != nil {
				catcher.Wrapf(err, "syncing project vars for project '%s'", pRef.Id)
				continue
			}
			if err := pVars.SetParamMappings(*pm); err != nil {
				catcher.Wrapf(err, "updating parameter mappings for project '%s'", pRef.Id)
				continue
			}

			// Double check that the project vars that were just synced agree
			// with the project vars in the DB when read out of Parameter Store.
			// This is mostly to cover inactive projects that may not have users
			// actively using them (and therefore the project vars never get
			// read out of Parameter Store to trigger a consistency check).
			// The consistency check happens internally within FindOne, so
			// the actual return value doesn't matter.
			_, _ = model.FindOneProjectVars(pRef.Id)
		}

		if !pRef.ParameterStoreGitHubAppSynced {
			ghAppAuth, err := model.GitHubAppAuthFindOne(pRef.Id)
			if err != nil {
				catcher.Wrapf(err, "finding GitHub App auth for project '%s'", pRef.Id)
				continue
			}
			if ghAppAuth != nil {
				grip.Info(message.Fields{
					"message":                 "syncing project GitHub app private key to Parameter Store",
					"existing_parameter_name": ghAppAuth.PrivateKeyParameter,
					"project_id":              pRef.Id,
					"is_repo_ref":             areRepoRefs,
					"epic":                    "DEVPROD-5552",
					"job":                     j.ID(),
				})
				if err := model.GitHubAppAuthUpsert(ghAppAuth); err != nil {
					catcher.Wrapf(err, "syncing GitHub app private key for project '%s' to Parameter Store", pRef.Id)
					continue
				}

				// Double check that the GitHub app private key that was just
				// synced agrees with the private key in the DB when read out of
				// Parameter Store. This is to cover inactive projects that may
				// not have users actively using them (and therefore the GitHub
				// app private key may never get read out of Parameter Store to
				// trigger a consistency check).
				// The consistency check happens internally within FindOne, so
				// the actual return value doesn't matter.
				_, _ = model.GitHubAppAuthFindOne(pRef.Id)
			}
		}
	}
	return catcher.Resolve()
}
