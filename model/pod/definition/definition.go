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
	ID string `bson:"_id"`
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
func (pd *PodDefinition) Insert(ctx context.Context) error {
	return db.Insert(ctx, Collection, pd)
}

// Replace updates the pod definition in the db if an entry already exists,
// overwriting the existing definition. If no definition exists, a new one is created.
func (pd *PodDefinition) Replace(ctx context.Context) error {
	_, err := db.Replace(ctx, Collection, ByID(pd.ID), pd)
	return err
}

// Remove removes the pod definition from the collection.
func (pd *PodDefinition) Remove(ctx context.Context) error {
	return db.Remove(ctx, Collection, ByID(pd.ID))
}

// UpdateLastAccessed updates the time this pod definition was last accessed to
// now.
func (pd *PodDefinition) UpdateLastAccessed(ctx context.Context) error {
	return UpdateOne(ctx, ByID(pd.ID), bson.M{
		"$set": bson.M{
			LastAccessedKey: time.Now(),
		},
	})
}

// PodDefinitionCache implements a cocoa.ECSPodDefinitionCache to cache pod
// definitions in the DB.
type PodDefinitionCache struct{}

// Put inserts a new pod definition; if an identical one already exists, this is
// a no-op.
func (pdc PodDefinitionCache) Put(ctx context.Context, item cocoa.ECSPodDefinitionItem) error {
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
			IDKey: primitive.NewObjectID().Hex(),
		},
	}
	if _, err := UpsertOne(ctx, idAndFamily, newPodDef); err != nil {
		return errors.Wrap(err, "upserting pod definition")
	}
	return nil
}

// Delete deletes a new pod definition by its external ID. If the pod definition
// does not exist, this is a no-op.
func (pdc PodDefinitionCache) Delete(ctx context.Context, externalID string) error {
	if err := db.Remove(ctx, Collection, bson.M{
		ExternalIDKey: externalID,
	}); err != nil {
		return errors.Wrapf(err, "deleting pod definition with external ID '%s'", externalID)
	}

	return nil
}

// PodDefinitionTag is the tag used to track pod definitions.
const PodDefinitionTag = "evergreen-tracked"

// GetTag returns the tag used for tracking cloud pod definitions.
func (pdc PodDefinitionCache) GetTag() string {
	return PodDefinitionTag
}
