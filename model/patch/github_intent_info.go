package patch

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GitHubIntentInfoCollection stores information about GitHub intents that failed to create patches.
const GitHubIntentInfoCollection = "github_intent_info"

// GitHubIntentInfo contains information about a GitHub intent that prevented it from creating a patch.
type GitHubIntentInfo struct {
	ID        string    `bson:"_id"`
	ProjectID string    `bson:"project_id"`
	IntentID  string    `bson:"intent_id"`
	Message   string    `bson:"message"`
	CreatedAt time.Time `bson:"created_at"`
}

// InsertGitHubIntentInfo stores immutable information about a GitHub intent that failed to create a patch.
func InsertGitHubIntentInfo(ctx context.Context, projectID, intentID, message string) (*GitHubIntentInfo, error) {
	if projectID == "" {
		return nil, errors.New("project ID cannot be empty")
	}
	if message == "" {
		return nil, errors.New("message cannot be empty")
	}
	intentInfo := &GitHubIntentInfo{
		ID:        primitive.NewObjectID().Hex(),
		ProjectID: projectID,
		IntentID:  intentID,
		Message:   message,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	if err := db.Insert(ctx, GitHubIntentInfoCollection, intentInfo); err != nil {
		return nil, err
	}
	return intentInfo, nil
}

// FindGitHubIntentInfo finds information about a GitHub intent processing error by ID.
func FindGitHubIntentInfo(ctx context.Context, id string) (*GitHubIntentInfo, error) {
	intentInfo := &GitHubIntentInfo{}
	err := db.FindOneQ(ctx, GitHubIntentInfoCollection, db.Query(bson.M{"_id": id}), intentInfo)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return intentInfo, err
}
