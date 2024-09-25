package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awsECS "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	sns "github.com/robbiet480/go.sns"
)

const (
	// SNS message types
	// See https://docs.aws.amazon.com/sns/latest/dg/sns-message-and-json-formats.html#http-header
	messageTypeSubscriptionConfirmation = "SubscriptionConfirmation"
	messageTypeNotification             = "Notification"
	messageTypeUnsubscribeConfirmation  = "UnsubscribeConfirmation"

	interruptionWarningType = "EC2 Spot Instance Interruption Warning"
	instanceStateChangeType = "EC2 Instance State-change Notification"

	ecsTaskStateChangeType              = "ECS Task State Change"
	ecsContainerInstanceStateChangeType = "ECS Container Instance State Change"
)

type baseSNS struct {
	queue amboy.Queue
	env   evergreen.Environment

	messageType string
	payload     sns.Payload
}

func makeBaseSNS(env evergreen.Environment, queue amboy.Queue) baseSNS {
	return baseSNS{
		env:   env,
		queue: queue,
	}
}

func (bsns *baseSNS) Parse(ctx context.Context, r *http.Request) error {
	bsns.messageType = r.Header.Get("x-amz-sns-message-type")
	payload := getSNSPayload(r.Context())
	if (payload == sns.Payload{}) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "payload not in context",
		}
	}

	bsns.payload = payload

	return nil
}

func (bsns *baseSNS) handleSNSConfirmation() (handled bool) {
	// Subscription/Unsubscription is a rare action that we will handle manually
	// and will be logged to splunk given the logging level.
	switch bsns.messageType {
	case messageTypeSubscriptionConfirmation:
		grip.Alert(message.Fields{
			"message":       "got AWS SNS subscription confirmation. Visit subscribe_url to confirm",
			"subscribe_url": bsns.payload.SubscribeURL,
			"topic_arn":     bsns.payload.TopicArn,
		})
		return true
	case messageTypeUnsubscribeConfirmation:
		grip.Alert(message.Fields{
			"message":         "got AWS SNS unsubscription confirmation. Visit unsubscribe_url to confirm",
			"unsubscribe_url": bsns.payload.UnsubscribeURL,
			"topic_arn":       bsns.payload.TopicArn,
		})
		return true
	default:
		return false
	}
}

type ec2SNS struct {
	baseSNS
}

func makeEC2SNS(env evergreen.Environment, queue amboy.Queue) gimlet.RouteHandler {
	return &ec2SNS{
		baseSNS: makeBaseSNS(env, queue),
	}
}

func (sns *ec2SNS) Factory() gimlet.RouteHandler {
	return &ec2SNS{
		baseSNS: makeBaseSNS(sns.env, sns.queue),
	}
}

func (sns *ec2SNS) Run(ctx context.Context) gimlet.Responder {
	if sns.handleSNSConfirmation() {
		return gimlet.NewJSONResponse(struct{}{})
	}

	switch sns.messageType {
	case messageTypeNotification:
		if err := sns.handleNotification(ctx); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":      "handling SNS notification",
				"notification": sns.payload.Message,
			}))
			return gimlet.NewJSONResponse(err)
		}
		return gimlet.NewJSONResponse(struct{}{})
	default:
		grip.Error(message.Fields{
			"message":         "got an unknown message type",
			"type":            sns.messageType,
			"payload_subject": sns.payload.Subject,
			"payload_message": sns.payload.Message,
			"payload_topic":   sns.payload.TopicArn,
		})
		return gimlet.NewTextErrorResponse(fmt.Sprintf("message type '%s' is not recognized", sns.messageType))
	}
}

type ec2EventBridgeNotification struct {
	EventTime  string         `json:"time"`
	DetailType string         `json:"detail-type"`
	Detail     ec2EventDetail `json:"detail"`
}

type ec2EventDetail struct {
	InstanceID string `json:"instance-id"`
	State      string `json:"state"`
}

func (sns *ec2SNS) handleNotification(ctx context.Context) error {
	notification := &ec2EventBridgeNotification{}
	if err := json.Unmarshal([]byte(sns.payload.Message), notification); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "unmarshalling notification").Error(),
		}
	}

	switch notification.DetailType {
	case interruptionWarningType:
		if err := sns.handleInstanceInterruptionWarning(ctx, notification.Detail.InstanceID); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrap(err, "processing interruption warning").Error(),
			}
		}
	case instanceStateChangeType:
		switch types.InstanceStateName(notification.Detail.State) {
		case types.InstanceStateNameRunning:
			if err := sns.handleInstanceRunning(ctx, notification.Detail.InstanceID, notification.EventTime); err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "processing running instance").Error(),
				}
			}
		case types.InstanceStateNameTerminated:
			if err := sns.handleInstanceTerminated(ctx, notification.Detail.InstanceID); err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "processing instance termination").Error(),
				}
			}
		case types.InstanceStateNameStopped, types.InstanceStateNameStopping:
			if err := sns.handleInstanceStopped(ctx, notification.Detail.InstanceID); err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "processing stopped instance").Error(),
				}
			}
		}
	default:
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("unknown detail type '%s'", notification.DetailType),
		}
	}

	return nil
}

func (sns *ec2SNS) handleInstanceInterruptionWarning(ctx context.Context, instanceID string) error {
	h, err := host.FindOneId(ctx, instanceID)
	if err != nil {
		return err
	}
	// don't make AWS keep retrying if we haven't heard of the host
	if h == nil {
		return nil
	}

	var instanceType string
	if len(h.Distro.ProviderSettingsList) > 0 {
		if stringVal, ok := h.Distro.ProviderSettingsList[0].Lookup("instance_type").StringValueOK(); ok {
			instanceType = stringVal
		}
	}
	existingHostCount, err := host.CountActiveHostsInDistro(ctx, h.Distro.Id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":               "database error counting running hosts by distro_id",
			"host_id":               h.Id,
			"distro_id":             h.Distro.Id,
			"instance_type":         instanceType,
			"missing_instance_type": instanceType == "",
		}))
		existingHostCount = -1
	}

	grip.Info(message.Fields{
		"message":               "got interruption warning from AWS",
		"distro":                h.Distro.Id,
		"running_host_count":    existingHostCount,
		"instance_type":         instanceType,
		"missing_instance_type": instanceType == "",
		"host_id":               h.Id,
	})

	return nil
}

func (sns *ec2SNS) handleInstanceTerminated(ctx context.Context, instanceID string) error {
	h, err := host.FindOneId(ctx, instanceID)
	if err != nil {
		return err
	}
	// don't make AWS keep retrying if we haven't heard of the host
	if h == nil {
		return nil
	}

	if h.Status == evergreen.HostTerminated {
		return nil
	}

	// The host is going to imminently stop work anyways. Decommissioning
	// ensures that even if the external state check does not terminate the
	// host, the host is eventually picked up for termination.
	if err := h.SetDecommissioned(ctx, evergreen.User, true, "SNS notification indicates host is terminated"); err != nil {
		return errors.Wrap(err, "decommissioning host")
	}

	if err := amboy.EnqueueUniqueJob(ctx, sns.queue, units.NewHostMonitoringCheckJob(sns.env, h, sns.payload.MessageId)); err != nil {
		return errors.Wrap(err, "enqueueing host external state check job")
	}

	return nil
}

func (sns *ec2SNS) handleInstanceRunning(ctx context.Context, instanceID, eventTimestamp string) error {
	h, err := host.FindOneId(ctx, instanceID)
	if err != nil {
		return err
	}
	if h == nil {
		return nil
	}

	runningTime, err := time.Parse(time.RFC3339, eventTimestamp)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "got malformed timestamp",
			"timestamp": eventTimestamp,
			"operation": "handleInstanceRunning",
			"host_id":   h.Id,
			"distro":    h.Distro.Id,
		}))
		runningTime = time.Now()
	}

	return errors.Wrap(h.SetBillingStartTime(ctx, runningTime), "setting billing start time")
}

// handleInstanceStopped handles an agent host when AWS reports that it is
// stopped. Agent hosts that are stopped externally are treated the same as an
// externally-terminated host.
func (sns *ec2SNS) handleInstanceStopped(ctx context.Context, instanceID string) error {
	h, err := host.FindOneId(ctx, instanceID)
	if err != nil {
		return err
	}
	if h == nil {
		return nil
	}

	// Ignore non-agent hosts (e.g. spawn hosts, host.create hosts).
	if h.UserHost || h.StartedBy != evergreen.User {
		return nil
	}

	if utility.StringSliceContains([]string{evergreen.HostStopped, evergreen.HostStopping, evergreen.HostTerminated}, h.Status) {
		grip.WarningWhen(utility.StringSliceContains([]string{evergreen.HostStopped, evergreen.HostStopping}, h.Status), message.Fields{
			"message":     "cannot handle unexpected host state: a host running tasks should never be stopped by Evergreen",
			"host_id":     h.Id,
			"host_status": h.Status,
		})
		return nil
	}

	// The host is going to imminently stop work anyways. Decommissioning
	// ensures that even if the external state check does not terminate the
	// host, the host is eventually picked up for termination.
	if err := h.SetDecommissioned(ctx, evergreen.User, true, "SNS notification indicates host is stopped"); err != nil {
		return errors.Wrap(err, "decommissioning host")
	}

	if err := amboy.EnqueueUniqueJob(ctx, sns.queue, units.NewHostMonitoringCheckJob(sns.env, h, sns.payload.MessageId)); err != nil {
		return errors.Wrap(err, "enqueueing host external state check job")
	}

	return nil
}

type ecsSNS struct {
	baseSNS
}

type ecsEventBridgeNotification struct {
	// DetailType is the type of ECS Event Bridge notification.
	DetailType string `json:"detail-type"`
	// Detail is the detailed contents of the notification. The actual contents
	// depend on the DetailType.
	Detail json.RawMessage `json:"detail"`
}

type ecsTaskEventDetail struct {
	TaskARN       string `json:"taskArn"`
	ClusterARN    string `json:"clusterArn"`
	LastStatus    string `json:"lastStatus"`
	DesiredStatus string `json:"desiredStatus"`
	StoppedReason string `json:"stoppedReason"`
}

type ecsContainerInstanceEventDetail struct {
	ContainerInstanceARN string `json:"containerInstanceArn"`
	ClusterARN           string `json:"clusterArn"`
	Status               string `json:"status"`
	StatusReason         string `json:"statusReason"`
}

func makeECSSNS(env evergreen.Environment, queue amboy.Queue) gimlet.RouteHandler {
	return &ecsSNS{
		baseSNS: makeBaseSNS(env, queue),
	}
}

func (sns *ecsSNS) Factory() gimlet.RouteHandler {
	return &ecsSNS{
		baseSNS: makeBaseSNS(sns.env, sns.queue),
	}
}

func (sns *ecsSNS) Run(ctx context.Context) gimlet.Responder {
	if sns.handleSNSConfirmation() {
		return gimlet.NewJSONResponse(struct{}{})
	}

	switch sns.messageType {
	case messageTypeNotification:
		notification := ecsEventBridgeNotification{}
		if err := json.Unmarshal([]byte(sns.payload.Message), &notification); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unmarshalling notification"))
		}

		if err := sns.handleNotification(ctx, notification); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":      "could not handle SNS notification",
				"notification": sns.payload.Message,
				"route":        "/hooks/aws/ecs",
			}))
			return gimlet.NewJSONResponse(err)
		}

		return gimlet.NewJSONResponse(struct{}{})
	default:
		grip.Error(message.Fields{
			"message":         "received an unknown message type",
			"type":            sns.messageType,
			"payload_subject": sns.payload.Subject,
			"payload_message": sns.payload.Message,
			"payload_topic":   sns.payload.TopicArn,
			"route":           "/hooks/aws/ecs",
		})
		return gimlet.NewTextErrorResponse(fmt.Sprintf("message type '%s' is not recognized", sns.messageType))
	}
}

func (sns *ecsSNS) handleNotification(ctx context.Context, notification ecsEventBridgeNotification) error {
	switch notification.DetailType {
	case ecsTaskStateChangeType:
		var detail ecsTaskEventDetail
		if err := json.Unmarshal(notification.Detail, &detail); err != nil {
			return errors.Wrap(err, "unmarshalling event details as ECS task event details")
		}
		p, err := pod.FindOneByExternalID(detail.TaskARN)
		if err != nil {
			return err
		}
		if p == nil {
			grip.Info(message.Fields{
				"message":        "found unexpected ECS task that did not match any known pod in Evergreen",
				"task":           detail.TaskARN,
				"cluster":        detail.ClusterARN,
				"last_status":    detail.LastStatus,
				"desired_status": detail.DesiredStatus,
				"route":          "/hooks/aws/ecs",
			})
			if err := sns.cleanupUnrecognizedPod(ctx, detail); err != nil {
				return errors.Wrapf(err, "cleaning up unrecognized pod '%s'", detail.TaskARN)
			}

			return nil
		}

		status := ecs.TaskStatus(detail.LastStatus).ToCocoaStatus()
		switch status {
		case cocoa.StatusStarting, cocoa.StatusRunning:
			// No-op because the pod is not considered running until the agent
			// contacts the app server.
			return nil
		case cocoa.StatusStopping:
			// No-op because the pod is preparing to stop but is not yet
			// stopped.
			return nil
		case cocoa.StatusStopped:
			reason := detail.StoppedReason
			if reason == "" {
				reason = "stopped due to unknown reason"
			}
			return sns.handleStoppedPod(ctx, p, fmt.Sprintf("SNS notification: %s", reason))
		default:
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("unrecognized status '%s'", detail.LastStatus),
			}
		}
	case ecsContainerInstanceStateChangeType:
		var detail ecsContainerInstanceEventDetail
		if err := json.Unmarshal(notification.Detail, &detail); err != nil {
			return errors.Wrap(err, "unmarshalling event details as ECS container instance event details")
		}
		if ecs.ContainerInstanceStatus(detail.Status) != ecs.ContainerInstanceStatusDraining {
			return nil
		}

		grip.Info(message.Fields{
			"message":            "got notification for draining container instance",
			"container_instance": detail.ContainerInstanceARN,
			"cluster":            detail.ClusterARN,
			"reason":             detail.StatusReason,
		})

		taskARNs, err := sns.listECSTasks(ctx, detail)
		if err != nil {
			return errors.Wrapf(err, "listing ECS tasks associated with container instance '%s'", detail.ContainerInstanceARN)
		}
		for _, taskARN := range taskARNs {
			p, err := pod.FindOneByExternalID(taskARN)
			if err != nil {
				return errors.Wrapf(err, "finding pod with external ID '%s'", taskARN)
			}
			if p == nil {
				grip.Error(message.Fields{
					"message": "Found an ECS task running in a draining container instance, but it's not associated with any known pod." +
						" This isn't supposed to happen, so this may be due to a race (i.e. the ECS task started before we could track it in the DB) or a bug (i.e. failing to properly track a pod that we started).",
					"task":               taskARN,
					"container_instance": detail.ContainerInstanceARN,
					"cluster":            detail.ClusterARN,
				})
				continue
			}
			var reason string
			if detail.StatusReason != "" {
				reason = detail.StatusReason
			} else {
				reason = "container instance is draining"
			}
			if err := sns.handleStoppedPod(ctx, p, reason); err != nil {
				return errors.Wrapf(err, "handling pods on draining container instance '%s'", detail.ContainerInstanceARN)
			}
		}
		return nil
	default:
		grip.Error(message.Fields{
			"message":         "received an unknown notification detail type",
			"message_type":    sns.messageType,
			"detail_type":     notification.DetailType,
			"payload_subject": sns.payload.Subject,
			"payload_message": sns.payload.Message,
			"payload_topic":   sns.payload.TopicArn,
			"route":           "/hooks/aws/ecs",
		})
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("unknown detail type '%s'", notification.DetailType),
		}
	}
}

// handleStoppedPod handles an SNS notification that a pod has been stopped in
// ECS.
func (sns *ecsSNS) handleStoppedPod(ctx context.Context, p *pod.Pod, reason string) error {
	if p.Status == pod.StatusDecommissioned || p.Status == pod.StatusTerminated {
		return nil
	}

	if err := p.UpdateStatus(pod.StatusDecommissioned, reason); err != nil {
		return err
	}

	if err := amboy.EnqueueUniqueJob(ctx, sns.queue, units.NewPodTerminationJob(p.ID, reason, utility.RoundPartOfMinute(0))); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not enqueue job to terminate pod from SNS notification",
			"pod_id":     p.ID,
			"pod_status": p.Status,
			"route":      "/hooks/aws/ecs",
		}))
	}

	return nil
}

func (sns *ecsSNS) cleanupUnrecognizedPod(ctx context.Context, detail ecsTaskEventDetail) error {
	status := ecs.TaskStatus(detail.DesiredStatus).ToCocoaStatus()
	if status == cocoa.StatusStopping || status == cocoa.StatusStopped {
		// The unrecognized pod is already cleaning up, so no-op.
		return nil
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flag for unrecognized pod cleanup")
	}
	if flags.UnrecognizedPodCleanupDisabled {
		return nil
	}

	// Spot check that the notification is actually for a pod in an
	// Evergreen-owned cluster.
	var isInManagedCluster bool
	for _, c := range sns.env.Settings().Providers.AWS.Pod.ECS.Clusters {
		if strings.Contains(detail.ClusterARN, c.Name) {
			isInManagedCluster = true
			break
		}
	}
	if !isInManagedCluster {
		return nil
	}

	c, err := cloud.MakeECSClient(ctx, sns.env.Settings())
	if err != nil {
		return errors.Wrap(err, "getting ECS client")
	}

	resources := cocoa.NewECSPodResources().
		SetCluster(detail.ClusterARN).
		SetTaskID(detail.TaskARN)

	statusInfo := cocoa.NewECSPodStatusInfo().SetStatus(status)

	podOpts := ecs.NewBasicPodOptions().
		SetClient(c).
		SetStatusInfo(*statusInfo).
		SetResources(*resources)

	p, err := ecs.NewBasicPod(podOpts)
	if err != nil {
		return errors.Wrap(err, "getting pod")
	}

	if err := p.Delete(ctx); err != nil {
		return errors.Wrap(err, "cleaning up pod resources")
	}

	grip.Info(message.Fields{
		"message":        "successfully cleaned up unrecognized pod",
		"task":           detail.TaskARN,
		"cluster":        detail.ClusterARN,
		"last_status":    detail.LastStatus,
		"desired_status": detail.DesiredStatus,
		"route":          "/hooks/aws/ecs",
	})

	return nil
}

// listECSTasks returns the ARNs of all ECS tasks running in the container
// instance associated with the event.
func (sns *ecsSNS) listECSTasks(ctx context.Context, details ecsContainerInstanceEventDetail) ([]string, error) {
	// Spot check that the notification is actually for a pod in an
	// Evergreen-owned cluster.
	var isInManagedCluster bool
	for _, c := range sns.env.Settings().Providers.AWS.Pod.ECS.Clusters {
		if strings.Contains(details.ClusterARN, c.Name) {
			isInManagedCluster = true
			break
		}
	}
	if !isInManagedCluster {
		return nil, nil
	}

	c, err := cloud.MakeECSClient(ctx, sns.env.Settings())
	if err != nil {
		return nil, errors.Wrap(err, "getting ECS client")
	}

	var token *string
	var taskARNs []string

	// The app server shouldn't be able to loop infinitely here since it's tied
	// to the request context, but just to be extra safe and ensure the loop has
	// a guaranteed exit condition, put a reasonable cap on the max number of
	// pages of tasks that a container instance could possibly return.
	const maxRealisticTaskPages = 100
	for i := 0; i < maxRealisticTaskPages; i++ {
		resp, err := c.ListTasks(ctx, &awsECS.ListTasksInput{
			Cluster:           aws.String(details.ClusterARN),
			ContainerInstance: aws.String(details.ContainerInstanceARN),
			NextToken:         token,
		})
		if err != nil {
			return nil, errors.Wrap(err, "listing ECS tasks")
		}
		if resp == nil {
			return nil, errors.New("listing ECS tasks returned no response")
		}

		taskARNs = append(taskARNs, resp.TaskArns...)
		token = resp.NextToken
		if token == nil {
			return taskARNs, nil
		}
	}

	return nil, errors.Errorf("hit max realistic number of pages of tasks (%d) for a single container instance, refusing to iterate through any more pages", maxRealisticTaskPages)
}
