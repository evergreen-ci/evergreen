package units

import (
	"context"
	"fmt"
	"time"

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
	UpstreamProjectID   string           `bson:"upstream_project_id" json:"upstream_project_id" yaml:"upstream_project_id"`
	DownstreamProjectID string           `bson:"downstream_project_id" json:"downstream_project_id" yaml:"downstream_project_id"`
	Commits             []model.Revision `bson:"commits" json:"commits" yaml:"commits"`
	IngestTime          time.Time        `bson:"ingest_time" json:"ingest_time" yaml:"ingest_time"`
	job.Base            `bson:"job_base" json:"job_base" yaml:"job_base"`
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
// downstream project for commits pushed to the upstream project. It runs one
// at a time with the downstream project's repotracker jobs.
func NewPushTriggerVersionsJob(msgID, upstreamProjectID, downstreamProjectID string, commits []model.Revision, ingestTime time.Time) amboy.Job {
	j := makePushTriggerVersionsJob()
	j.UpstreamProjectID = upstreamProjectID
	j.DownstreamProjectID = downstreamProjectID
	j.Commits = commits
	j.IngestTime = ingestTime
	j.SetID(fmt.Sprintf("%s:%s:%s:%s", pushTriggerVersionsJobName, msgID, upstreamProjectID, downstreamProjectID))
	j.SetScopes([]string{versionCreationScope(downstreamProjectID)})
	return j
}

func (j *pushTriggerVersionsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// Look up the downstream project the same way the webhook does, rather
	// than loading its merged project ref.
	downstreamProjects, err := trigger.FindPushTriggeredProjects(ctx, j.UpstreamProjectID)
	if err != nil {
		j.AddError(err)
		return
	}
	for _, downstream := range downstreamProjects {
		if downstream.Id != j.DownstreamProjectID {
			continue
		}
		j.AddError(errors.Wrapf(trigger.TriggerDownstreamProjectForPush(ctx, j.UpstreamProjectID, downstream, j.Commits, j.IngestTime, trigger.TriggerDownstreamVersion),
			"triggering downstream project '%s' for push to project '%s'", j.DownstreamProjectID, j.UpstreamProjectID))
		return
	}
}
