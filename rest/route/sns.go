package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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

	ecsTaskStateChangeType = "ECS Task State Change"
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

func (sns *baseSNS) Parse(ctx context.Context, r *http.Request) error {
	sns.messageType = r.Header.Get("x-amz-sns-message-type")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "problem reading body")
	}
	if err = json.Unmarshal(body, &sns.payload); err != nil {
		return errors.Wrap(err, "problem unmarshalling payload")
	}

	if err = sns.payload.VerifyPayload(); err != nil {
		msg := "AWS SNS message failed validation"
		grip.Error(message.WrapError(err, message.Fields{
			"message": msg,
			"payload": sns.payload,
		}))
		return errors.Wrap(err, msg)
	}

	return nil
}

func (sns *baseSNS) handleSNSConfirmation() (handled bool) {
	// Subscription/Unsubscription is a rare action that we will handle manually
	// and will be logged to splunk given the logging level.
	switch sns.messageType {
	case messageTypeSubscriptionConfirmation:
		grip.Alert(message.Fields{
			"message":       "got AWS SNS subscription confirmation. Visit subscribe_url to confirm",
			"subscribe_url": sns.payload.SubscribeURL,
			"topic_arn":     sns.payload.TopicArn,
		})
		return true
	case messageTypeUnsubscribeConfirmation:
		grip.Alert(message.Fields{
			"message":         "got AWS SNS unsubscription confirmation. Visit unsubscribe_url to confirm",
			"unsubscribe_url": sns.payload.UnsubscribeURL,
			"topic_arn":       sns.payload.TopicArn,
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
				"message":      "problem handling SNS notification",
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
			Message:    errors.Wrap(err, "problem unmarshalling notification").Error(),
		}
	}

	switch notification.DetailType {
	case interruptionWarningType:
		if err := sns.handleInstanceInterruptionWarning(ctx, notification.Detail.InstanceID); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrap(err, "problem processing interruption warning").Error(),
			}
		}
	case instanceStateChangeType:
		switch notification.Detail.State {
		case ec2.InstanceStateNameTerminated:
			if err := sns.handleInstanceTerminated(ctx, notification.Detail.InstanceID); err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "problem processing instance termination").Error(),
				}
			}
		case ec2.InstanceStateNameStopped, ec2.InstanceStateNameStopping:
			if err := sns.handleInstanceStopped(ctx, notification.Detail.InstanceID); err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "problem processing stopped instance").Error(),
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
	h, err := host.FindOneId(instanceID)
	if err != nil {
		return err
	}
	// don't make AWS keep retrying if we haven't heard of the host
	if h == nil {
		return nil
	}

	instanceType := "Empty Distro.ProviderSettingsList, unable to get instance_type"
	if len(h.Distro.ProviderSettingsList) > 0 {
		if stringVal, ok := h.Distro.ProviderSettingsList[0].Lookup("instance_type").StringValueOK(); ok {
			instanceType = stringVal
		}
	}
	existingHostCount, err := host.CountRunningHosts(h.Distro.Id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "database error counting running hosts by distro_id",
			"host_id":       h.Id,
			"distro_id":     h.Distro.Id,
			"instance_type": instanceType,
		}))
		existingHostCount = -1
	}

	grip.Info(message.Fields{
		"message":            "got interruption warning from AWS",
		"distro":             h.Distro.Id,
		"running_host_count": existingHostCount,
		"instance_type":      instanceType,
		"host_id":            h.Id,
	})

	return nil
}

func (sns *ec2SNS) handleInstanceTerminated(ctx context.Context, instanceID string) error {
	h, err := host.FindOneId(instanceID)
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

	if err := amboy.EnqueueUniqueJob(ctx, sns.queue, units.NewHostMonitorExternalStateJob(sns.env, h, sns.payload.MessageId)); err != nil {
		return err
	}

	return nil
}

// handleInstanceStopped handles an agent host when AWS reports that it is
// stopped. Agent hosts that are stopped externally are treated the same as an
// externally-terminated host.
func (sns *ec2SNS) handleInstanceStopped(ctx context.Context, instanceID string) error {
	h, err := host.FindOneId(instanceID)
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

	if err := amboy.EnqueueUniqueJob(ctx, sns.queue, units.NewHostMonitorExternalStateJob(sns.env, h, sns.payload.MessageId)); err != nil {
		return err
	}

	return nil
}

type ecsSNS struct {
	baseSNS

	makeECSClient func(*evergreen.Settings) (cocoa.ECSClient, error)
}

type ecsEventBridgeNotification struct {
	DetailType string         `json:"detail-type"`
	Detail     ecsEventDetail `json:"detail"`
}

type ecsEventDetail struct {
	TaskARN       string `json:"taskArn"`
	ClusterARN    string `json:"clusterArn"`
	LastStatus    string `json:"lastStatus"`
	DesiredStatus string `json:"desiredStatus"`
	StoppedReason string `json:"stoppedReason"`
}

func makeECSSNS(env evergreen.Environment, queue amboy.Queue) gimlet.RouteHandler {
	return &ecsSNS{
		baseSNS:       makeBaseSNS(env, queue),
		makeECSClient: cloud.MakeECSClient,
	}
}

func (sns *ecsSNS) Factory() gimlet.RouteHandler {
	return &ecsSNS{
		baseSNS:       makeBaseSNS(sns.env, sns.queue),
		makeECSClient: sns.makeECSClient,
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
		p, err := data.FindPodByExternalID(notification.Detail.TaskARN)
		if err != nil {
			return err
		}
		if p == nil {
			grip.Info(message.Fields{
				"message":        "found unexpected ECS task that did not match any known pod in Evergreen",
				"task":           notification.Detail.TaskARN,
				"cluster":        notification.Detail.ClusterARN,
				"last_status":    notification.Detail.LastStatus,
				"desired_status": notification.Detail.DesiredStatus,
				"route":          "/hooks/aws/ecs",
			})
			if err := sns.cleanupUnrecognizedPod(ctx, notification.Detail); err != nil {
				return errors.Wrapf(err, "cleaning up unrecognized pod '%s'", notification.Detail.TaskARN)
			}

			return nil
		}

		status := ecs.TaskStatus(notification.Detail.LastStatus).ToCocoaStatus()
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
			reason := notification.Detail.StoppedReason
			if reason == "" {
				reason = "stopped due to unknown reason"
			}
			return sns.handleStoppedPod(ctx, p, fmt.Sprintf("SNS notification: %s", reason))
		default:
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("unrecognized status '%s'", notification.Detail.LastStatus),
			}
		}
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
func (sns *ecsSNS) handleStoppedPod(ctx context.Context, p *model.APIPod, reason string) error {
	if p.Status == model.PodStatusDecommissioned || p.Status == model.PodStatusTerminated {
		return nil
	}
	id := utility.FromStringPtr(p.ID)

	if err := data.UpdatePodStatus(id, p.Status, model.PodStatusDecommissioned); err != nil {
		return err
	}

	if err := amboy.EnqueueUniqueJob(ctx, sns.queue, units.NewPodTerminationJob(id, reason, utility.RoundPartOfMinute(0))); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "could not enqueue job to terminate pod from SNS notification",
			"pod_id":     id,
			"pod_status": p.Status,
			"route":      "/hooks/aws/ecs",
		}))
	}

	return nil
}

func (sns *ecsSNS) cleanupUnrecognizedPod(ctx context.Context, details ecsEventDetail) error {
	status := ecs.TaskStatus(details.DesiredStatus).ToCocoaStatus()
	if status == cocoa.StatusStopping || status == cocoa.StatusStopped {
		// The unrecognized pod is already cleaning up, so no-op.
		return nil
	}

	flags, err := evergreen.GetServiceFlags()
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
		if strings.Contains(details.ClusterARN, c.Name) {
			isInManagedCluster = true
			break
		}
	}
	if !isInManagedCluster {
		return nil
	}

	c, err := sns.makeECSClient(sns.env.Settings())
	if err != nil {
		return errors.Wrap(err, "getting ECS client")
	}
	defer c.Close(ctx)

	resources := cocoa.NewECSPodResources().
		SetCluster(details.ClusterARN).
		SetTaskID(details.TaskARN)

	statusInfo := cocoa.NewECSPodStatusInfo().SetStatus(status)

	podOpts := ecs.NewBasicECSPodOptions().
		SetClient(c).
		SetStatusInfo(*statusInfo).
		SetResources(*resources)

	p, err := ecs.NewBasicECSPod(podOpts)
	if err != nil {
		return errors.Wrap(err, "getting pod")
	}

	if err := p.Delete(ctx); err != nil {
		return errors.Wrap(err, "cleaning up pod resources")
	}

	grip.Info(message.Fields{
		"message":        "successfully cleaned up unrecognized pod",
		"task":           details.TaskARN,
		"cluster":        details.ClusterARN,
		"last_status":    details.LastStatus,
		"desired_status": details.DesiredStatus,
		"route":          "/hooks/aws/ecs",
	})

	return nil
}
