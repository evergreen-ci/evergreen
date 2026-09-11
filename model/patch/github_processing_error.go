package patch

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// GitHubIntentProcessingErrorCollection stores errors that prevented GitHub intents from creating patches.
const GitHubIntentProcessingErrorCollection = "github_intent_processing_errors"

// GitHubIntentProcessingError contains an error that prevented a GitHub intent from creating a patch.
type GitHubIntentProcessingError struct {
	ID        mgobson.ObjectId `bson:"_id"`
	ProjectID string           `bson:"project_id"`
	Message   string           `bson:"message"`
}

// InsertGitHubIntentProcessingError stores an immutable GitHub intent processing error.
func InsertGitHubIntentProcessingError(ctx context.Context, projectID, message string) (*GitHubIntentProcessingError, error) {
	if projectID == "" {
		return nil, errors.New("project ID cannot be empty")
	}
	if message == "" {
		return nil, errors.New("message cannot be empty")
	}
	processingError := &GitHubIntentProcessingError{
		ID:        mgobson.NewObjectId(),
		ProjectID: projectID,
		Message:   message,
	}
	if err := db.Insert(ctx, GitHubIntentProcessingErrorCollection, processingError); err != nil {
		return nil, err
	}
	return processingError, nil
}

// FindGitHubIntentProcessingError finds a GitHub intent processing error by ID.
func FindGitHubIntentProcessingError(ctx context.Context, id mgobson.ObjectId) (*GitHubIntentProcessingError, error) {
	processingError := &GitHubIntentProcessingError{}
	err := db.FindOneQ(ctx, GitHubIntentProcessingErrorCollection, db.Query(bson.M{"_id": id}), processingError)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return processingError, err
}
