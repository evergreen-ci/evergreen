package notification

type emailPayload struct {
	Headers map[string]string `bson:"headers"`
	Subject string            `bson:"subject"`
	Body    []byte            `bson:"body"`
}

type githubStatusAPIPayload struct {
	Status      string `bson:"status"`
	Context     string `bson:"context"`
	Description string `bson:"description"`
	URL         string `bson:"url"`
}

// TODO
// This is a copy of message.JiraIssue, but with bson tags.
// Is there a way around this?
type jiraIssuePayload struct {
	Summary     string   `bson:"summary"`
	Description string   `bson:"description"`
	Reporter    string   `bson:"reporter"`
	Assignee    string   `bson:"assignee"`
	Type        string   `bson:"type"`
	Components  []string `bson:"components"`
	Labels      []string `bson:"labels"`
	// ... other fields
	Fields map[string]string `bson:"fields"`
}
