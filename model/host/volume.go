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
	DisplayName      string    `bson:"display_name" json:"display_name"`
	CreatedBy        string    `bson:"created_by" json:"created_by"`
	Type             string    `bson:"type" json:"type"`
	Size             int       `bson:"size" json:"size"`
	AvailabilityZone string    `bson:"availability_zone" json:"availability_zone"`
	Expiration       time.Time `bson:"expiration" json:"expiration"`
	NoExpiration     bool      `bson:"no_expiration" json:"no_expiration"`
	CreationDate     time.Time `bson:"created_at" json:"created_at"`
	Host             string    `bson:"host" json:"host"`
}

// Insert a volume into the volumes collection.
func (v *Volume) Insert() error {
	v.CreationDate = time.Now()
	return db.Insert(VolumesCollection, v)
}

func (v *Volume) SetHost(id string) error {
	err := db.Update(VolumesCollection,
		bson.M{VolumeIDKey: v.ID},
		bson.M{"$set": bson.M{VolumeHostKey: id}})

	if err != nil {
		return errors.WithStack(err)
	}

	v.Host = id
	return nil
}

func UnsetVolumeHost(id string) error {
	return errors.WithStack(db.Update(VolumesCollection,
		bson.M{VolumeIDKey: id},
		bson.M{"$unset": bson.M{VolumeHostKey: true}}))
}

func (v *Volume) SetDisplayName(displayName string) error {
	err := db.Update(VolumesCollection,
		bson.M{VolumeIDKey: v.ID},
		bson.M{"$set": bson.M{VolumeDisplayNameKey: displayName}})
	if err != nil {
		return errors.WithStack(err)
	}
	v.DisplayName = displayName
	return nil
}

func (v *Volume) SetSize(size int) error {
	err := db.Update(VolumesCollection,
		bson.M{VolumeIDKey: v.ID},
		bson.M{"$set": bson.M{VolumeSizeKey: size}})
	if err != nil {
		return errors.WithStack(err)
	}
	v.Size = size
	return nil
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
		bson.M{"$set": bson.M{VolumeExpirationKey: v.Expiration}})
}

func (v *Volume) SetNoExpiration(noExpiration bool) error {
	v.NoExpiration = noExpiration

	return db.UpdateId(
		VolumesCollection,
		v.ID,
		bson.M{"$set": bson.M{VolumeNoExpirationKey: noExpiration}})
}

func FindVolumesToDelete(expirationTime time.Time) ([]Volume, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{VolumeExpirationKey: bson.M{"$lte": expirationTime}},
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
				"hosts": bson.M{
					"$filter": bson.M{
						"input": "$hosts",
						"as":    "host",
						"cond":  bson.M{"$ne": []string{"$$host.status", evergreen.HostTerminated}},
					},
				},
				VolumeExpirationKey: 1,
			},
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

func ValidateVolumeCanBeAttached(volumeID string) (*Volume, error) {
	volume, err := FindVolumeByID(volumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get volume '%s'", volumeID)
	}
	if volume == nil {
		return nil, errors.Errorf("volume '%s' does not exist", volumeID)
	}
	var sourceHost *Host
	sourceHost, err = FindHostWithVolume(volumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get source host for volume '%s'", volumeID)
	}
	if sourceHost != nil {
		return nil, errors.Errorf("volume '%s' is already attached to host '%s'", volumeID, sourceHost.Id)
	}
	return volume, nil
}
