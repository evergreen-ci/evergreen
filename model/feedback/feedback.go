package feedback

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
)

const Collection = "feedback"

type FeedbackSubmission struct {
	Type        string           `json:"type" bson:"type"`
	User        string           `json:"user,omitempty" bson:"user,omitempty"`
	SubmittedAt time.Time        `json:"submitted_at" bson:"submitted_at"`
	Questions   []QuestionAnswer `json:"questions" bson:"questions"`
}

type QuestionAnswer struct {
	ID     string `json:"id" bson:"id"`
	Prompt string `json:"prompt" bson:"prompt"`
	Answer string `json:"answer" bson:"answer"`
}

func (s *FeedbackSubmission) Insert() error {
	return db.Insert(Collection, s)
}

func (s *FeedbackSubmission) FindOfType(t string) ([]FeedbackSubmission, error) {
	return nil, nil
}
