package host

import (
	"context"
	"sort"
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
	Size             int32     `bson:"size" json:"size"`
	Throughput       int32     `bson:"throughput,omitempty" json:"throughput,omitempty"`
	IOPS             int32     `bson:"iops,omitempty" json:"iops,omitempty"`
	AvailabilityZone string    `bson:"availability_zone" json:"availability_zone"`
	Expiration       time.Time `bson:"expiration" json:"expiration"`
	NoExpiration     bool      `bson:"no_expiration" json:"no_expiration"`
	CreationDate     time.Time `bson:"created_at" json:"created_at"`
	Host             string    `bson:"host,omitempty" json:"host"`
	HomeVolume       bool      `bson:"home_volume" json:"home_volume"`
	Migrating        bool      `bson:"migrating" json:"migrating"`
}

// Insert a volume into the volumes collection.
func (v *Volume) Insert(ctx context.Context) error {
	v.CreationDate = time.Now()
	return db.Insert(ctx, VolumesCollection, v)
}

func (v *Volume) SetHost(ctx context.Context, id string) error {
	err := db.Update(ctx, VolumesCollection,
		bson.M{VolumeIDKey: v.ID},
		bson.M{"$set": bson.M{VolumeHostKey: id}})

	if err != nil {
		return errors.WithStack(err)
	}

	v.Host = id
	return nil
}

func UnsetVolumeHost(ctx context.Context, id string) error {
	return errors.WithStack(db.Update(ctx, VolumesCollection,
		bson.M{VolumeIDKey: id},
		bson.M{"$unset": bson.M{VolumeHostKey: true}}))
}

func (v *Volume) SetDisplayName(ctx context.Context, displayName string) error {
	err := db.Update(ctx, VolumesCollection,
		bson.M{VolumeIDKey: v.ID},
		bson.M{"$set": bson.M{VolumeDisplayNameKey: displayName}})
	if err != nil {
		return errors.WithStack(err)
	}
	v.DisplayName = displayName
	return nil
}

func (v *Volume) SetSize(ctx context.Context, size int32) error {
	err := db.Update(ctx, VolumesCollection,
		bson.M{VolumeIDKey: v.ID},
		bson.M{"$set": bson.M{VolumeSizeKey: size}})
	if err != nil {
		return errors.WithStack(err)
	}
	v.Size = size
	return nil
}

func (v *Volume) SetMigrating(ctx context.Context, migrating bool) error {
	err := db.Update(ctx, VolumesCollection,
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
func (v *Volume) Remove(ctx context.Context) error {
	return db.Remove(
		ctx,
		VolumesCollection,
		bson.M{
			VolumeIDKey: v.ID,
		},
	)
}

func (v *Volume) SetExpiration(ctx context.Context, expiration time.Time) error {
	v.Expiration = expiration

	return db.UpdateIdContext(
		ctx,
		VolumesCollection,
		v.ID,
		bson.M{"$set": bson.M{VolumeExpirationKey: v.Expiration}})
}

func (v *Volume) SetNoExpiration(ctx context.Context, noExpiration bool) error {
	v.NoExpiration = noExpiration

	return db.UpdateIdContext(
		ctx,
		VolumesCollection,
		v.ID,
		bson.M{"$set": bson.M{VolumeNoExpirationKey: noExpiration}})
}

func FindVolumesToDelete(ctx context.Context, expirationTime time.Time) ([]Volume, error) {
	q := bson.M{
		VolumeExpirationKey: bson.M{"$lte": expirationTime},
		VolumeHostKey:       bson.M{"$exists": false},
	}

	return findVolumes(ctx, q)
}

// FindVolumesWithTerminatedHost finds volumes that are attached to a host we have marked as terminated, indicating the
// volume is stuck in an invalid state.
func FindVolumesWithTerminatedHost(ctx context.Context) ([]Volume, error) {
	match := bson.M{
		"$match": bson.M{
			VolumeHostKey: bson.M{"$exists": true},
		}}
	lookup := bson.M{
		"$lookup": bson.M{
			"from":         Collection,
			"localField":   "host",
			"foreignField": "_id",
			"as":           "host_doc",
		}}
	matchTerminatedHosts := bson.M{
		"$match": bson.M{
			"host_doc.status": evergreen.HostTerminated,
		}}
	project := bson.M{"$project": bson.M{"host_doc": 0}}
	pipeline := []bson.M{match, lookup, matchTerminatedHosts, project}
	volumes := []Volume{}
	if err := db.Aggregate(ctx, VolumesCollection, pipeline, &volumes); err != nil {
		return nil, err
	}
	return volumes, nil
}

func FindUnattachedExpirableVolumes(ctx context.Context) ([]Volume, error) {
	q := bson.M{
		NoExpirationKey: bson.M{"$ne": true},
		VolumeHostKey:   bson.M{"$exists": false},
	}

	return findVolumes(ctx, q)
}

// FindVolumeByID finds a volume by its ID field.
func FindVolumeByID(ctx context.Context, id string) (*Volume, error) {
	return FindOneVolume(ctx, bson.M{VolumeIDKey: id})
}

type volumeSize struct {
	TotalVolumeSize int `bson:"total"`
}

func FindTotalVolumeSizeByUser(ctx context.Context, user string) (int, error) {
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
	err := db.Aggregate(ctx, VolumesCollection, pipeline, &out)
	if err != nil || len(out) == 0 {
		return 0, err
	}

	return out[0].TotalVolumeSize, nil
}

func FindVolumesWithNoExpirationToExtend(ctx context.Context) ([]Volume, error) {
	query := bson.M{
		VolumeNoExpirationKey: true,
		VolumeHostKey:         bson.M{"$exists": false},
		VolumeExpirationKey:   bson.M{"$lte": time.Now().Add(24 * time.Hour)},
	}

	return findVolumes(ctx, query)
}

func FindVolumesByUser(ctx context.Context, userID string) ([]Volume, error) {
	query := bson.M{
		VolumeCreatedByKey: userID,
	}
	return findVolumes(ctx, query)
}

func FindSortedVolumesByUser(ctx context.Context, userID string) ([]Volume, error) {
	volumes, err := FindVolumesByUser(ctx, userID)
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

func ValidateVolumeCanBeAttached(ctx context.Context, volumeID string) (*Volume, error) {
	volume, err := FindVolumeByID(ctx, volumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting volume '%s'", volumeID)
	}
	if volume == nil {
		return nil, errors.Errorf("volume '%s' does not exist", volumeID)
	}
	var sourceHost *Host
	sourceHost, err = FindHostWithVolume(ctx, volumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "getting host with volume '%s' attached", volumeID)
	}
	if sourceHost != nil {
		return nil, errors.Errorf("volume '%s' is already attached to host '%s'", volumeID, sourceHost.Id)
	}
	return volume, nil
}

func CountNoExpirationVolumesForUser(ctx context.Context, userID string) (int, error) {
	return db.Count(ctx, VolumesCollection, bson.M{
		VolumeNoExpirationKey: true,
		VolumeCreatedByKey:    userID,
	})
}
