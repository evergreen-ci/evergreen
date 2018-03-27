package notification

type EmailPayload struct {
	Headers map[string]string `bson:"headers"`
	Subject string            `bson:"subject"`
	Body    string            `bson:"body"`
}

type GithubStatusAPIPayload struct {
	Status      string `bson:"status"`
	Context     string `bson:"context"`
	Description string `bson:"description"`
	URL         string `bson:"url"`
}
