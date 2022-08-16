package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	podDefinitionCleanupJobName = "pod-definition-cleanup"
	podDefinitionTTL            = 30 * 24 * time.Hour
)

func init() {
	registry.AddJobType(podDefinitionCleanupJobName, func() amboy.Job {
		return makePodDefinitionCleanupJob()
	})
}

type podDefinitionCleanupJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	env       evergreen.Environment
	settings  evergreen.Settings
	ecsClient cocoa.ECSClient
	podDefMgr cocoa.ECSPodDefinitionManager
}

func makePodDefinitionCleanupJob() *podDefinitionCleanupJob {
	j := &podDefinitionCleanupJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podDefinitionCleanupJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewPodDefinitionCleanupJob creates a job that cleans up pod definitions that
// have not been used recently.
func NewPodDefinitionCleanupJob(id string) amboy.Job {
	j := makePodDefinitionCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", podDefinitionCleanupJobName, id))
	return j
}

func (j *podDefinitionCleanupJob) Run(ctx context.Context) {
	defer func() {
		j.MarkComplete()

		if j.ecsClient != nil {
			j.AddError(errors.Wrap(j.ecsClient.Close(ctx), "closing ECS client"))
		}
	}()
	if err := j.populate(); err != nil {
		j.AddError(err)
		return
	}

	podDefs, err := definition.FindByLastAccessedBefore(podDefinitionTTL, -1)
	if err != nil {
		j.AddError(err)
		return
	}

	for _, podDef := range podDefs {
		if podDef.ExternalID == "" {
			if err := podDef.Remove(); err != nil {
				j.AddError(errors.Wrapf(err, "deleting pod definition '%s' which is missing an external ID", podDef.ID))
				continue
			}

			grip.Info(message.Fields{
				"message":        "cleaned up pod definition that has no corresponding external ID",
				"pod_definition": podDef.ID,
				"last_accessed":  podDef.LastAccessed,
				"job":            j.ID(),
			})
			continue
		}

		if err := j.podDefMgr.DeletePodDefinition(ctx, podDef.ExternalID); err != nil {
			j.AddError(errors.Wrapf(err, "deleting pod definition '%s'", podDef.ID))
			continue
		}

		grip.Info(message.Fields{
			"message":        "successfully cleaned up pod definition",
			"pod_definition": podDef.ID,
			"external_id":    podDef.ExternalID,
			"last_accessed":  podDef.LastAccessed,
			"job":            j.ID(),
		})
	}
}

func (j *podDefinitionCleanupJob) populate() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	// Use the latest service flags instead of those cached in the environment.
	settings := *j.env.Settings()
	if err := settings.ServiceFlags.Get(j.env); err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	j.settings = settings

	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(&settings)
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	if j.podDefMgr == nil {
		podDefMgr, err := cloud.MakeECSPodDefinitionManager(j.ecsClient, nil)
		if err != nil {
			return errors.Wrap(err, "initializing ECS pod definition manager")
		}
		j.podDefMgr = podDefMgr
	}

	return nil
}
