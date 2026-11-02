package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const pushTriggerVersionsJobName = "push-trigger-versions"

func init() {
	registry.AddJobType(pushTriggerVersionsJobName, func() amboy.Job { return makePushTriggerVersionsJob() })
}

type pushTriggerVersionsJob struct {
	Push                trigger.UpstreamPush `bson:"push" json:"push" yaml:"push"`
	DownstreamProjectID string               `bson:"downstream_project_id" json:"downstream_project_id" yaml:"downstream_project_id"`
	job.Base            `bson:"job_base" json:"job_base" yaml:"job_base"`

	processor func(context.Context, trigger.ProcessorArgs) (*model.Version, error)
}

func makePushTriggerVersionsJob() *pushTriggerVersionsJob {
	return &pushTriggerVersionsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    pushTriggerVersionsJobName,
				Version: 0,
			},
		},
	}
}

// NewPushTriggerVersionsJob creates a job that creates versions in the
// downstream project for the pushed commits. It runs one at a time with the
// downstream project's repotracker jobs.
func NewPushTriggerVersionsJob(msgID string, push trigger.UpstreamPush, downstreamProjectID string) amboy.Job {
	j := makePushTriggerVersionsJob()
	j.Push = push
	j.DownstreamProjectID = downstreamProjectID
	j.SetID(fmt.Sprintf("%s:%s:%s:%s", pushTriggerVersionsJobName, msgID, push.ProjectID, downstreamProjectID))
	j.SetScopes([]string{versionCreationScope(downstreamProjectID)})
	return j
}

func (j *pushTriggerVersionsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.processor == nil {
		j.processor = trigger.TriggerDownstreamVersion
	}

	downstream, err := model.FindMergedProjectRef(ctx, j.DownstreamProjectID, "", true)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding downstream project '%s'", j.DownstreamProjectID))
		return
	}
	if downstream == nil || !downstream.Enabled {
		return
	}
	j.AddError(errors.Wrapf(trigger.TriggerDownstreamProjectForPush(ctx, j.Push, *downstream, j.processor),
		"triggering downstream project '%s' for push to project '%s'", j.DownstreamProjectID, j.Push.ProjectID))
}
