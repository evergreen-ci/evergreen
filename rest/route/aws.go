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
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	sns "github.com/robbiet480/go.sns"
)

const (
	messageTypeSubscriptionConfirmation = "SubscriptionConfirmation"
	messageTypeNotification             = "Notification"
	messageTypeUnsubscribeConfirmation  = "UnsubscribeConfirmation"
	interruptionWarningType             = "EC2 Spot Instance Interruption Warning"
	instanceStateChangeType             = "EC2 Instance State-change Notification"
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
		return aws.handleNotification(ctx)
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

func (aws *awsSns) handleNotification(ctx context.Context) gimlet.Responder {
	notification := &eventBridgeNotification{}
	if err := json.Unmarshal([]byte(aws.payload.Message), notification); err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "problem unmarshalling notification"))
	}

	switch notification.DetailType {
	case interruptionWarningType:
		if err := aws.handleInstanceInterruptionWarning(ctx, notification.Detail.InstanceID); err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrap(err, "problem processing interruption warning"))
		}

	case instanceStateChangeType:
		if notification.Detail.State == ec2.InstanceStateNameTerminated {
			if err := aws.handleInstanceTerminated(ctx, notification.Detail.InstanceID); err != nil {
				return gimlet.NewJSONErrorResponse(errors.Wrap(err, "problem processing instance termination"))
			}
		}

	default:
		return gimlet.NewJSONErrorResponse(errors.Errorf("unknown detail type '%s'", notification.DetailType))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (aws *awsSns) handleInstanceInterruptionWarning(ctx context.Context, instanceID string) error {
	h, err := aws.sc.FindHostById(instanceID)
	if err != nil {
		return err
	}

	if h.Status != evergreen.HostRunning {
		return nil
	}

	if skipEarlyTermination(h) {
		return nil
	}

	// return success on duplicate job errors so AWS won't keep retrying
	_ = aws.queue.Put(ctx, units.NewHostTerminationJob(aws.env, h, true, "got interruption warning"))

	return nil
}

// skipEarlyTermination decides if we should terminate the instance now or wait for AWS to do it.
// See https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-interruptions.html#billing-for-interrupted-spot-instances
func skipEarlyTermination(h *host.Host) bool {
	// Let AWS terminate if we're within the first hour
	if time.Now().Sub(h.StartTime) < time.Hour {
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

	if h.Status == evergreen.HostTerminated {
		return nil
	}

	// return success on duplicate job errors so AWS won't keep retrying
	_ = aws.queue.Put(ctx, units.NewHostMonitorExternalStateJob(aws.env, h, aws.payload.MessageId))

	return nil
}
