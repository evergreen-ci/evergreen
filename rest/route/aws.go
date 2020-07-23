package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	sns "github.com/robbiet480/go.sns"
)

const (
	messageTypeSubscriptionConfirmation = "SubscriptionConfirmation"
	messageTypeNotification             = "Notification"
	messageTypeUnsubscribeConfirmation  = "UnsubscribeConfirmation"
)

type awsSns struct {
	sc          data.Connector
	messageType string
	payload     sns.Payload
}

func makeAwsSnsRoute(sc data.Connector) gimlet.RouteHandler {
	return &awsSns{
		sc: sc,
	}
}

func (aws *awsSns) Factory() gimlet.RouteHandler {
	return &awsSns{
		sc: aws.sc,
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
		// TODO: handle the message
		grip.Info(message.Fields{
			"message":         "got an AWS SNS notification",
			"payload_subject": aws.payload.Subject,
			"payload_message": aws.payload.Message,
			"payload_topic":   aws.payload.TopicArn,
		})
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
