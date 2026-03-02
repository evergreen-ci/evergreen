package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
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

	instanceStateChangeType = "EC2 Instance State-change Notification"
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
