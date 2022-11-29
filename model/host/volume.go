package host

import (
	"sort"
	"time"

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
	Throughput       int       `bson:"throughput,omitempty" json:"throughput,omitempty"`
	IOPS             int       `bson:"iops,omitempty" json:"iops,omitempty"`
	AvailabilityZone string    `bson:"availability_zone" json:"availability_zone"`
	Expiration       time.Time `bson:"expiration" json:"expiration"`
	NoExpiration     bool      `bson:"no_expiration" json:"no_expiration"`
	CreationDate     time.Time `bson:"created_at" json:"created_at"`
	Host             string    `bson:"host,omitempty" json:"host"`
	HomeVolume       bool      `bson:"home_volume" json:"home_volume"`
	Migrating        bool      `bson:"migrating" json:"migrating"`
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

func (v *Volume) SetMigrating(migrating bool) error {
	err := db.Update(VolumesCollection,
		bson.M{VolumeIDKey: v.ID},
		bson.M{"$set": bson.M{VolumeMigratingKey: migrating}})
	if err != nil {
		return errors.WithStack(err)
	}
	v.Migrating = migrating
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
	q := bson.M{
		VolumeExpirationKey: bson.M{"$lte": expirationTime},
		VolumeHostKey:       bson.M{"$exists": false},
	}

	return findVolumes(q)
}

func FindUnattachedExpirableVolumes() ([]Volume, error) {
	q := bson.M{
		NoExpirationKey: bson.M{"$ne": true},
		VolumeHostKey:   bson.M{"$exists": false},
	}

	return findVolumes(q)
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

func FindVolumesWithNoExpirationToExtend() ([]Volume, error) {
	query := bson.M{
		VolumeNoExpirationKey: true,
		VolumeHostKey:         bson.M{"$exists": false},
		VolumeExpirationKey:   bson.M{"$lte": time.Now().Add(24 * time.Hour)},
	}

	return findVolumes(query)
}

func FindVolumesByUser(userID string) ([]Volume, error) {
	query := bson.M{
		VolumeCreatedByKey: userID,
	}
	return findVolumes(query)
}

func FindSortedVolumesByUser(userID string) ([]Volume, error) {
	volumes, err := FindVolumesByUser(userID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting volumes for user '%s'", userID)
	}

	sort.SliceStable(volumes, func(i, j int) bool {
		// sort order: mounted < not mounted, expiration time asc
		volumeI := volumes[i]
		volumeJ := volumes[j]
		isMountedI := volumeI.Host == ""
		isMountedJ := volumeJ.Host == ""
		if isMountedI == isMountedJ {
			return volumeI.Expiration.Before(volumeJ.Expiration)
		}
		return isMountedJ
	})
	return volumes, nil
}

func ValidateVolumeCanBeAttached(volumeID string) (*Volume, error) {
	volume, err := FindVolumeByID(volumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting volume '%s'", volumeID)
	}
	if volume == nil {
		return nil, errors.Errorf("volume '%s' does not exist", volumeID)
	}
	var sourceHost *Host
	sourceHost, err = FindHostWithVolume(volumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting host with volume '%s' attached", volumeID)
	}
	if sourceHost != nil {
		return nil, errors.Errorf("volume '%s' is already attached to host '%s'", volumeID, sourceHost.Id)
	}
	return volume, nil
}

func CountNoExpirationVolumesForUser(userID string) (int, error) {
	return db.Count(VolumesCollection, bson.M{
		VolumeNoExpirationKey: true,
		VolumeCreatedByKey:    userID,
	})
}
