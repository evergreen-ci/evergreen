package notification

import "github.com/mongodb/grip/message"

type EmailPayload struct {
	Headers map[string]string `bson:"headers"`
	Subject string            `bson:"subject"`
	Body    string            `bson:"body"`
}

type GithubStatusAPIPayload struct {
	State       message.GithubState `bson:"state"`
	Context     string              `bson:"context"`
	Description string              `bson:"description"`
	URL         string              `bson:"url"`
}
