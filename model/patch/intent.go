package patch

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// Intent represents an intent to create a patch build and is processed by an amboy queue.
type Intent interface {
	// ID returns an identifier such that the tuple
	// (intent type, ID()) is unique in the collection.
	ID() string

	// Insert inserts a patch intent in the database.
	Insert() error

	// SetProcessed should be called by an amboy queue after creating a patch from an intent.
	SetProcessed() error

	// IsProcessed returns whether a patch exists for this intent.
	IsProcessed() bool

	// GetType returns the patch intent, e.g., GithubIntentType.
	GetType() string

	// NewPatch creates a patch from the intent
	NewPatch() *Patch

	// Finalize indicates whether or not the patch created from this
	// intent should be finalized
	ShouldFinalizePatch() bool

	// GetAlias defines the variants and tasks this intent should run on.
	GetAlias() string

	// RequesterIdentity supplies a valid requester type, that is recorded
	// in patches, versions, builds, and tasks to denote the origin of the
	// patch
	RequesterIdentity() string
}

// FindIntent returns an intent of the specified type from the database
func FindIntent(id, intentType string) (Intent, error) {
	intent, ok := GetIntent(intentType)
	if !ok {
		return nil, errors.Errorf("no intent of type '%s' registered", intentType)
	}

	err := db.FindOneQ(IntentCollection, db.Query(bson.M{"_id": id}), intent)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return intent, nil
}
