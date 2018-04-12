package notification

import "github.com/mongodb/grip/message"

type SlackPayload struct {
	Body        string                    `bson:"body"`
	Attachments []message.SlackAttachment `bson:"attachments"`
}

type evergreenWebhookPayload struct {
	id     string `bson:"notification_id"`
	url    string `bson:"url"`
	secret []byte `bson:"secret"`
	body   []byte `bson:"body"`
}
