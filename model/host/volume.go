package host

import (
	"github.com/evergreen-ci/evergreen/db"
	"go.mongodb.org/mongo-driver/bson"
)

type Volume struct {
	ID        string `bson:"_id" json:"id"`
	CreatedBy string `bson:"created_by" json:"created_by"`
	Type      string `bson:"type" json:"type"`
	Size      int    `bson:"size" json:"size"`
}

// Insert a volume into the volumes collection.
func (v *Volume) Insert() error {
	return db.Insert(VolumesCollection, v)
}

// Remove a volume from the volumes collection.
func (v *Volume) Remove() error {
	return db.Remove(
		VolumesCollection,
		bson.M{
			IdKey: v.ID,
		},
	)
}

// FindVolumeByID finds a volume by its ID field.
func FindVolumeByID(id string) (*Volume, error) {
	return FindOneVolume(db.Query(bson.M{VolumeIDKey: id}))
}
