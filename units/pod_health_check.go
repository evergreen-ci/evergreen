package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	podHealthCheckJobName = "pod-health-check"

	podReachabilityThreshold = 10 * time.Minute
)

func init() {
	registry.AddJobType(podHealthCheckJobName, func() amboy.Job {
		return makePodHealthCheckJob()
	})
}

type podHealthCheckJob struct {
	PodID    string `bson:"pod_id" json:"pod_id" yaml:"pod_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	env       evergreen.Environment
	pod       *pod.Pod
	ecsClient cocoa.ECSClient
	ecsPod    cocoa.ECSPod
}

func makePodHealthCheckJob() *podHealthCheckJob {
	j := &podHealthCheckJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podHealthCheckJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewPodHealthCheckJob creates a job to check the health of the pod according
// to the cloud provider.
func NewPodHealthCheckJob(podID string, ts time.Time) amboy.Job {
	j := makePodHealthCheckJob()

	j.PodID = podID
	j.SetScopes([]string{fmt.Sprintf("%s.%s", podHealthCheckJobName, podID), podLifecycleScope(podID)})
	j.SetEnqueueAllScopes(true)
	j.SetID(fmt.Sprintf("%s.%s.%s", podHealthCheckJobName, podID, ts.Format(TSFormat)))

	return j
}

func (j *podHealthCheckJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "getting admin settings"))
		return
	}
	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message":   "monitor is disabled",
			"operation": j.Type().Name,
			"impact":    "skipping pod health check",
			"mode":      "degraded",
		})
		return
	}

	defer func() {
		if j.ecsClient != nil {
			j.AddError(errors.Wrap(j.ecsClient.Close(ctx), "closing ECS client"))
		}
	}()

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.pod.Status != pod.StatusStarting && j.pod.Status != pod.StatusRunning {
		grip.Info(message.Fields{
			"message":           "pod is not in a state where its current cloud pod status can be checked",
			"pod":               j.PodID,
			"status":            j.pod.Status,
			"last_communicated": j.pod.TimeInfo.LastCommunicated,
			"job":               j.ID(),
		})
		return
	}

	if time.Since(j.pod.TimeInfo.LastCommunicated) <= podReachabilityThreshold {
		grip.Info(message.Fields{
			"message":           "pod is considered healthy because last communication was within reachability threshold",
			"pod":               j.PodID,
			"status":            j.pod.Status,
			"last_communicated": j.pod.TimeInfo.LastCommunicated,
			"job":               j.ID(),
		})
		return
	}

	info, err := j.ecsPod.LatestStatusInfo(ctx)
	if err != nil {
		grip.Warning(message.Fields{
			"message":           "unable to get cloud pod's status info",
			"pod":               j.PodID,
			"status":            j.pod.Status,
			"last_communicated": j.pod.TimeInfo.LastCommunicated,
			"job":               j.ID(),
			"err":               err.Error(),
		})
		return
	}

	switch info.Status {
	case cocoa.StatusStarting, cocoa.StatusRunning:
		grip.Info(message.Fields{
			"message": "cloud pod is healthy",
			"pod":     j.PodID,
			"status":  info.Status,
			"job":     j.ID(),
		})
	case cocoa.StatusStopping, cocoa.StatusStopped:
		grip.Info(message.Fields{
			"message": "cloud pod is unhealthy",
			"pod":     j.PodID,
			"status":  info.Status,
			"job":     j.ID(),
		})

		terminationJob := NewPodTerminationJob(j.PodID, fmt.Sprintf("pod health check detected status '%s'", info.Status), utility.RoundPartOfMinute(0))
		if err := amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), terminationJob); err != nil {
			j.AddError(errors.Wrap(err, "enqueueing job to terminate unhealthy pod"))
			return
		}
	default:
		grip.Warning(message.Fields{
			"message": "unable to determine pod health because it is in an unhandled state",
			"pod":     j.PodID,
			"status":  info.Status,
			"job":     j.ID(),
		})
	}
}

func (j *podHealthCheckJob) populateIfUnset() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.pod == nil {
		p, err := pod.FindOneByID(j.PodID)
		if err != nil {
			return errors.Wrapf(err, "finding pod '%s'", j.PodID)
		}
		if p == nil {
			return errors.Errorf("pod '%s' not found", j.PodID)
		}
		j.pod = p
	}

	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(j.env.Settings())
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	if j.ecsPod == nil {
		ecsPod, err := cloud.ExportECSPod(j.pod, j.ecsClient, nil)
		if err != nil {
			return errors.Wrap(err, "exporting pod resources")
		}
		j.ecsPod = ecsPod
	}

	return nil
}
