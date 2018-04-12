package notification

import "github.com/mongodb/grip/message"

type SlackPayload struct {
	Body        string                    `bson:"body"`
	Attachments []message.SlackAttachment `bson:"attachments"`
}
