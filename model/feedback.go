package model

import (
	"context"
	"time"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/evergreen-ci/evergreen/db"
)

const FeedbackCollection = "feedback"

var (
	FeedbackTypeKey = bsonutil.MustHaveTag(FeedbackSubmission{}, "Type")
)

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

func (s *FeedbackSubmission) Insert(ctx context.Context) error {
	return db.Insert(ctx, FeedbackCollection, s)
}

func FindFeedbackOfType(ctx context.Context, t string) ([]FeedbackSubmission, error) {
	out := []FeedbackSubmission{}
	query := db.Query(bson.M{FeedbackTypeKey: t})
	err := db.FindAllQContext(ctx, FeedbackCollection, query, &out)
	if err != nil {
		return nil, errors.Wrap(err, "finding feedback documents")
	}
	return out, nil
}
