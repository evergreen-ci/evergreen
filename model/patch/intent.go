package patch

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// Intent represents an intent to create a patch build and is processed by an amboy queue.
type Intent interface {
	// ID returns a unique identifier for the patch. Should
	// correspond to the _id for the patch in database.
	ID() string

	// Insert inserts a patch intent in the database.
	Insert(ctx context.Context) error

	// SetProcessed should be called by an amboy queue after creating a patch from an intent.
	SetProcessed(ctx context.Context) error

	// IsProcessed returns whether a patch exists for this intent.
	IsProcessed() bool

	// GetType returns the patch intent, e.g., GithubIntentType.
	GetType() string

	// NewPatch creates a patch from the intent
	NewPatch() *Patch

	// Finalize indicates whether or not the patch created from this
	// intent should be finalized
	ShouldFinalizePatch() bool

	// RepeatPreviousPatchDefinition returns true if we should use the same tasks/variants as a previous patch.
	// Returns patch ID if specified, otherwise we use the latest patch.
	RepeatPreviousPatchDefinition() (string, bool)

	// RepeatFailedTasksAndVariants returns true if we should use the failed tasks/variants from a previous patch.
	// Returns patch ID if specified, otherwise we use the latest patch.
	RepeatFailedTasksAndVariants() (string, bool)

	// GetAlias defines the variants and tasks this intent should run on.
	GetAlias() string

	// RequesterIdentity supplies a valid requester type, that is recorded
	// in patches, versions, builds, and tasks to denote the origin of the
	// patch
	RequesterIdentity() string

	// GetCalledBy indicates whether the intent was created automatically
	// by Evergreen or manually by the user.
	GetCalledBy() string
}

// FindIntent returns an intent of the specified type from the database
func FindIntent(ctx context.Context, id, intentType string) (Intent, error) {
	intent, ok := GetIntent(intentType)
	if !ok {
		return nil, errors.Errorf("no intent of type '%s' registered", intentType)
	}

	err := db.FindOneQ(ctx, IntentCollection, db.Query(bson.M{"_id": id}), intent)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return intent, nil
}
