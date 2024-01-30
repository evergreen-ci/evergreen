package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	spawnHostStartRetryLimit = 10
	spawnhostStartName       = "spawnhost-start"
)

func init() {
	registry.AddJobType(spawnhostStartName, func() amboy.Job {
		return makeSpawnhostStartJob()
	})
}

type spawnhostStartJob struct {
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeSpawnhostStartJob() *spawnhostStartJob {
	j := &spawnhostStartJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostStartName,
				Version: 0,
			},
		},
	}
	return j
}

// NewSpawnhostStartJob returns a job to start a stopped spawn host.
func NewSpawnhostStartJob(h *host.Host, user, ts string) amboy.Job {
	j := makeSpawnhostStartJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostStartName, user, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = h.Id
	j.CloudHostModification.UserID = user
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(spawnHostStartRetryLimit),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

func (j *spawnhostStartJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	startCloudHost := func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error {
		// kim: TODO: figure out if span has naming conventions (e.g. snake case)
		// kim: TODO: test
		ctx, span := tracer.Start(ctx, "start-spawn-host")
		defer span.End()

		if err := mgr.StartInstance(ctx, h, user); err != nil {
			event.LogHostStartError(h.Id, err.Error())
			span.SetStatus(codes.Error, "error starting host")
			// kim: TODO: figure out if these host attributes end up in the
			// exception, and what that actually entails
			span.RecordError(err, trace.WithAttributes(j.hostAttributes(h)...), trace.WithStackTrace(true))
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error starting spawn host",
				"host_id":  h.Id,
				"host_tag": h.Tag,
				"distro":   h.Distro.Id,
				"user":     user,
			}))
			return errors.Wrap(err, "starting spawn host")
		}

		event.LogHostStartSucceeded(h.Id)
		grip.Info(message.Fields{
			"message":  "started spawn host",
			"host_id":  h.Id,
			"host_tag": h.Tag,
			"distro":   h.Distro.Id,
			"user":     user,
		})

		return nil
	}
	if err := j.CloudHostModification.modifyHost(ctx, startCloudHost); err != nil {
		j.AddError(err)
		return
	}
}
