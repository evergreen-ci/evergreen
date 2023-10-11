package units

import (
	"context"
	"fmt"
	"strconv"
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
	tagClient cocoa.TagClient
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
	if err := j.populate(ctx); err != nil {
		j.AddError(err)
		return
	}

	cleanupLimit := j.settings.PodLifecycle.MaxPodDefinitionCleanupRate
	numDeleted, err := j.cleanupStrandedPodDefinitions(ctx, cleanupLimit)
	j.AddError(errors.Wrap(err, "cleaning up stranded pod definitions"))
	cleanupLimit -= numDeleted
	if cleanupLimit <= 0 {
		return
	}

	_, err = j.cleanupStalePodDefinitions(ctx, cleanupLimit)
	j.AddError(errors.Wrap(err, "cleaning up stale pod definitions"))
}

func (j *podDefinitionCleanupJob) populate(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	// Use the latest service flags instead of those cached in the environment.
	settings := *j.env.Settings()
	if err := settings.ServiceFlags.Get(ctx); err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	j.settings = settings

	if j.tagClient == nil {
		client, err := cloud.MakeTagClient(ctx, &settings)
		if err != nil {
			return errors.Wrap(err, "initializing tag client")
		}
		j.tagClient = client
	}
	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(ctx, &settings)
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

func (j *podDefinitionCleanupJob) cleanupStrandedPodDefinitions(ctx context.Context, limit int) (numDeleted int, err error) {
	podDefIDs, err := cloud.GetFilteredResourceIDs(ctx, j.tagClient, []string{cloud.PodDefinitionResourceFilter}, map[string][]string{
		definition.PodDefinitionTag: {strconv.FormatBool(false)},
	}, limit)
	if err != nil {
		err = errors.Wrap(err, "getting stranded pod definitions")
		j.AddError(err)
		return 0, err
	}

	catcher := grip.NewBasicCatcher()
	for _, podDefID := range podDefIDs {
		if err := j.podDefMgr.DeletePodDefinition(ctx, podDefID); err != nil {
			catcher.Wrapf(err, "deleting pod definition '%s'", podDefID)
			continue
		}

		numDeleted++

		grip.Info(message.Fields{
			"message":     "successfully cleaned up stranded pod definition",
			"external_id": podDefID,
			"job":         j.ID(),
		})
	}
	return numDeleted, catcher.Resolve()
}

func (j *podDefinitionCleanupJob) cleanupStalePodDefinitions(ctx context.Context, limit int) (numDeleted int, err error) {
	podDefs, err := definition.FindByLastAccessedBefore(podDefinitionTTL, limit)
	if err != nil {
		return 0, errors.Wrapf(err, "finding pod definitions last accessed before %s", time.Now().Add(-podDefinitionTTL))
	}

	catcher := grip.NewBasicCatcher()
	for _, podDef := range podDefs {
		if podDef.ExternalID == "" {
			if err := podDef.Remove(); err != nil {
				catcher.Wrapf(err, "deleting pod definition '%s' which is missing an external ID", podDef.ID)
				continue
			}

			numDeleted++
			grip.Info(message.Fields{
				"message":        "cleaned up pod definition that has no corresponding external ID",
				"pod_definition": podDef.ID,
				"last_accessed":  podDef.LastAccessed,
				"job":            j.ID(),
			})
			continue
		}

		if err := j.podDefMgr.DeletePodDefinition(ctx, podDef.ExternalID); err != nil {
			catcher.Wrapf(err, "deleting pod definition '%s'", podDef.ID)
			continue
		}

		numDeleted++
		grip.Info(message.Fields{
			"message":        "successfully cleaned up pod definition",
			"pod_definition": podDef.ID,
			"external_id":    podDef.ExternalID,
			"last_accessed":  podDef.LastAccessed,
			"job":            j.ID(),
		})
	}

	return numDeleted, catcher.Resolve()
}
