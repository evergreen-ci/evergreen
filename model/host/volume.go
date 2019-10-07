package host

import (
	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

type Volume struct {
	ID        string `bson:"_id" json:"id"`
	CreatedBy string `bson:"created_by" json:"created_by"`
	Type      string `bson:"type" json:"type"`
	Size      int    `bson:"size" json:"size"`
}

// Insert a volume into the volumes collection
func (v *Volume) Insert() error {
	return db.Insert(VolumesCollection, v)
}

// Remove a volume from the volumes collection
func (v *Volume) Remove() error {
	return db.Remove(
		VolumesCollection,
		bson.M{
			IdKey: v.ID,
		},
	)
}

// Find a volume by its ID field
func FindVolumeByID(id string) (*Volume, error) {
	volume := &Volume{}
	err := db.FindOneQ(
		VolumesCollection,
		db.Query(bson.D{{Key: VolumeIDKey, Value: id}}),
		volume,
	)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return volume, err
}
