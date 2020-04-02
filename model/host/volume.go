package host

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type Volume struct {
	ID               string    `bson:"_id" json:"id"`
	CreatedBy        string    `bson:"created_by" json:"created_by"`
	Type             string    `bson:"type" json:"type"`
	Size             int       `bson:"size" json:"size"`
	AvailabilityZone string    `bson:"availability_zone" json:"availability_zone"`
	Expiration       time.Time `bson:"expiration" json:"expiration"`
}

// Insert a volume into the volumes collection.
func (v *Volume) Insert() error {
	return db.Insert(VolumesCollection, v)
}

// Remove a volume from the volumes collection.
// Note this shouldn't be used when you want to
// remove from AWS itself.
func (v *Volume) Remove() error {
	return db.Remove(
		VolumesCollection,
		bson.M{
			VolumeIDKey: v.ID,
		},
	)
}

func (v *Volume) SetExpiration(expiration time.Time) error {
	v.Expiration = expiration

	return db.UpdateId(
		VolumesCollection,
		v.ID,
		bson.M{VolumeExpirationKey: v.Expiration})
}

func FindVolumesToDelete() ([]Volume, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{VolumeExpirationKey: bson.M{"$lte": time.Now()}},
		},
		{
			"$lookup": bson.M{
				"from":         Collection,
				"localField":   "_id",
				"foreignField": "volumes.volume_id",
				"as":           "hosts",
			},
		},
		{
			"$project": bson.M{
				"hosts": bson.M{"$filter": bson.M{
					"input": "$hosts",
					"as":    "host",
					"cond":  bson.M{"$ne": []string{"$$host.status", evergreen.HostTerminated}},
				},
				}},
		},
		{
			"$match": bson.M{"$expr": bson.M{"$eq": bson.A{bson.M{"$size": "$hosts"}, 0}}},
		},
	}

	volumes := []Volume{}
	err := db.Aggregate(VolumesCollection, pipeline, &volumes)
	if err != nil {
		return nil, errors.Wrap(err, "can't find volumes to delete")
	}

	return volumes, nil
}

// FindVolumeByID finds a volume by its ID field.
func FindVolumeByID(id string) (*Volume, error) {
	return FindOneVolume(bson.M{VolumeIDKey: id})
}

type volumeSize struct {
	TotalVolumeSize int `bson:"total"`
}

func FindTotalVolumeSizeByUser(user string) (int, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			VolumeCreatedByKey: user,
		}},
		{"$group": bson.M{
			"_id":   "123",
			"total": bson.M{"$sum": "$" + VolumeSizeKey},
		}},
	}

	out := []volumeSize{}
	err := db.Aggregate(VolumesCollection, pipeline, &out)
	if err != nil || len(out) == 0 {
		return 0, err
	}

	return out[0].TotalVolumeSize, nil
}
