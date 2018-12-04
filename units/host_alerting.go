package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const hostAlertingName = "host-alerting"

func init() {
	registry.AddJobType(hostAlertingName, func() amboy.Job {
		return makeHostAlerting()
	})
}

type hostAlertingJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	host   *host.Host
	logger grip.Journaler
}

func makeHostAlerting() *hostAlertingJob {
	j := &hostAlertingJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostAlertingName,
				Version: 0,
			},
		},
		logger: logging.MakeGrip(grip.GetSender()),
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostAlertingJob(h host.Host, ts string) amboy.Job {
	job := makeHostAlerting()

	job.host = &h
	job.HostID = h.Id

	job.SetID(fmt.Sprintf("%s.%s.%s", hostAlertingName, job.HostID, ts))

	return job
}

func (j *hostAlertingJob) Run(_ context.Context) {
	defer j.MarkComplete()

	if j.logger == nil {
		j.logger = logging.MakeGrip(grip.GetSender())
	}

	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if h == nil {
			j.AddError(errors.Errorf("unable to retrieve host %s", j.HostID))
			return
		}
		j.host = h
	}

	j.monitorLongRunningTasks()
}

func (j *hostAlertingJob) monitorLongRunningTasks() {
	const noticeThreshold = 24 * time.Hour
	const errorThreshold = 96 * time.Hour

	runningTask, err := task.FindOne(task.ById(j.host.RunningTask))
	if err != nil {
		j.AddError(err)
		return
	}
	if runningTask == nil {
		return
	}
	elapsed := time.Since(runningTask.StartTime)
	msg := message.Fields{
		"message":     "host running task for too long",
		"op":          hostAlertingName,
		"op_id":       j.ID(),
		"host":        j.HostID,
		"distro":      j.host.Distro.Id,
		"task":        runningTask.Id,
		"elapsed":     elapsed.String(),
		"elapsed_raw": elapsed,
	}
	if util.IsZeroTime(runningTask.StartTime) {
		return
	}
	j.logger.NoticeWhen(elapsed > noticeThreshold && elapsed <= errorThreshold, msg)
	j.logger.ErrorWhen(elapsed > errorThreshold, msg)
}
