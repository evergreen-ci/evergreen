package definition

import (
	"context"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PodDefinition represents a template definition for a pod kept in external
// storage.
type PodDefinition struct {
	// ID is the unique identifier for this document.
	ID string `bson:"id,omitempty"`
	// ExternalID is the identifier for the template definition in external
	// storage.
	ExternalID string `bson:"external_id,omitempty"`
	// Family is the family name of the pod definition stored in the cloud
	// provider.
	Family string `bson:"family,omitempty"`
	// LastAccessed is the timestamp for the last time this pod definition was
	// used.
	LastAccessed time.Time `bson:"last_accessed,omitempty"`
}

// Insert inserts the pod definition into the collection.
func (pd *PodDefinition) Insert() error {
	return db.Insert(Collection, pd)
}

// PodDefinitionCache implements a cocoa.ECSPodDefinitionCache to cache pod
// definitions in the DB.
type PodDefinitionCache struct{}

// Put inserts a new pod definition; if an identical one already exists, this is
// a no-op.
func (pdc PodDefinitionCache) Put(_ context.Context, item cocoa.ECSPodDefinitionItem) error {
	family := utility.FromStringPtr(item.DefinitionOpts.Name)
	idAndFamily := bson.M{
		ExternalIDKey: item.ID,
		FamilyKey:     family,
	}
	newPodDef := bson.M{
		"$set": bson.M{
			ExternalIDKey:   item.ID,
			FamilyKey:       family,
			LastAccessedKey: time.Now(),
		},
		"$setOnInsert": bson.M{
			IDKey: primitive.NewObjectID().String(),
		},
	}
	if _, err := UpsertOne(idAndFamily, newPodDef); err != nil {
		return errors.Wrap(err, "upserting pod definition")
	}
	return nil
}
