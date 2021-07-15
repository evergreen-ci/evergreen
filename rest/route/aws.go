package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
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
)

type awsSns struct {
	sc    data.Connector
	queue amboy.Queue
	env   evergreen.Environment

	messageType string
	payload     sns.Payload
}

func makeAwsSnsRoute(sc data.Connector, env evergreen.Environment, queue amboy.Queue) gimlet.RouteHandler {
	return &awsSns{
		sc:    sc,
		env:   env,
		queue: queue,
	}
}

func (aws *awsSns) Factory() gimlet.RouteHandler {
	return &awsSns{
		sc:    aws.sc,
		env:   aws.env,
		queue: aws.queue,
	}
}

func (aws *awsSns) Parse(ctx context.Context, r *http.Request) error {
	aws.messageType = r.Header.Get("x-amz-sns-message-type")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "problem reading body")
	}
	if err = json.Unmarshal(body, &aws.payload); err != nil {
		return errors.Wrap(err, "problem unmarshalling payload")
	}

	if err = aws.payload.VerifyPayload(); err != nil {
		msg := "AWS SNS message failed validation"
		grip.Error(message.WrapError(err, message.Fields{
			"message": msg,
			"payload": aws.payload,
		}))
		return errors.Wrap(err, msg)
	}

	return nil
}

func (aws *awsSns) Run(ctx context.Context) gimlet.Responder {
	// Subscription/Unsubscription is a rare action that we will handle manually and will be logged to splunk given the logging level.
	switch aws.messageType {
	case messageTypeSubscriptionConfirmation:
		grip.Alert(message.Fields{
			"message":       "got AWS SNS subscription confirmation. Visit subscribe_url to confirm",
			"subscribe_url": aws.payload.SubscribeURL,
			"topic_arn":     aws.payload.TopicArn,
		})
	case messageTypeUnsubscribeConfirmation:
		grip.Alert(message.Fields{
			"message":         "got AWS SNS unsubscription confirmation. Visit unsubscribe_url to confirm",
			"unsubscribe_url": aws.payload.SubscribeURL,
			"topic_arn":       aws.payload.TopicArn,
		})
	case messageTypeNotification:
		if err := aws.handleNotification(ctx); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":      "problem handling SNS notification",
				"notification": aws.payload.Message,
			}))
			return gimlet.NewJSONResponse(err)
		}
	default:
		grip.Error(message.Fields{
			"message":         "got an unknown message type",
			"type":            aws.messageType,
			"payload_subject": aws.payload.Subject,
			"payload_message": aws.payload.Message,
			"payload_topic":   aws.payload.TopicArn,
		})
		return gimlet.NewTextErrorResponse(fmt.Sprintf("message type '%s' is not recognized", aws.messageType))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type eventBridgeNotification struct {
	DetailType string      `json:"detail-type"`
	Detail     eventDetail `json:"detail"`
}
type eventDetail struct {
	InstanceID string `json:"instance-id"`
	State      string `json:"state"`
}

func (aws *awsSns) handleNotification(ctx context.Context) error {
	notification := &eventBridgeNotification{}
	if err := json.Unmarshal([]byte(aws.payload.Message), notification); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem unmarshalling notification").Error(),
		}
	}

	switch notification.DetailType {
	case interruptionWarningType:
		if err := aws.handleInstanceInterruptionWarning(ctx, notification.Detail.InstanceID); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrap(err, "problem processing interruption warning").Error(),
			}
		}

	case instanceStateChangeType:
		switch notification.Detail.State {
		case ec2.InstanceStateNameTerminated:
			if err := aws.handleInstanceTerminated(ctx, notification.Detail.InstanceID); err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "problem processing instance termination").Error(),
				}
			}
		case ec2.InstanceStateNameStopped:
			if err := aws.handleInstanceStopped(ctx, notification.Detail.InstanceID); err != nil {
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

func (aws *awsSns) handleInstanceInterruptionWarning(ctx context.Context, instanceID string) error {
	h, err := aws.sc.FindHostById(instanceID)
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
		grip.Warning(errors.Wrap(err, "database error counting running hosts by h.Distro.Id"))
		existingHostCount = -1
	}

	grip.Info(message.Fields{
		"message":            "got interruption warning from AWS",
		"distro":             h.Distro.Id,
		"running_host_count": existingHostCount,
		"instance_type":      instanceType,
		"host_id":            h.Id,
	})

	if h.Status == evergreen.HostTerminated {
		return nil
	}

	if skipEarlyTermination(h) {
		return nil
	}

	// return success on duplicate job errors so AWS won't keep retrying
	terminationJob := units.NewHostTerminationJob(aws.env, h, true, "got interruption warning")
	if err := amboy.EnqueueUniqueJob(ctx, aws.queue, terminationJob); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message":          "could not enqueue host termination job",
			"host_id":          h.Id,
			"enqueue_job_type": terminationJob.Type(),
		}))
	}

	return nil
}

// skipEarlyTermination decides if we should terminate the instance now or wait for AWS to do it.
// See https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-interruptions.html#billing-for-interrupted-spot-instances
func skipEarlyTermination(h *host.Host) bool {
	// Let AWS terminate if we're within the first hour
	if utility.IsZeroTime(h.StartTime) || time.Now().Sub(h.StartTime) < time.Hour {
		return true
	}

	// Let AWS terminate Windows hosts themselves.
	if h.Distro.IsWindows() {
		return true
	}

	return false
}

func (aws *awsSns) handleInstanceTerminated(ctx context.Context, instanceID string) error {
	h, err := aws.sc.FindHostById(instanceID)
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

	if err := amboy.EnqueueUniqueJob(ctx, aws.queue, units.NewHostMonitorExternalStateJob(aws.env, h, aws.payload.MessageId)); err != nil {
		return err
	}

	return nil
}

// handleInstanceStopped handles an agent host when AWS reports that it is
// stopped. Agent hosts that are stopped externally are treated the same as an
// externally-terminated host.
func (aws *awsSns) handleInstanceStopped(ctx context.Context, instanceID string) error {
	h, err := aws.sc.FindHostById(instanceID)
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

	if err := amboy.EnqueueUniqueJob(ctx, aws.queue, units.NewHostMonitorExternalStateJob(aws.env, h, aws.payload.MessageId)); err != nil {
		return err
	}

	return nil
}
