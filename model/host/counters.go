package host

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func (h *Host) IncTaskCount() error {
	query := bson.M{
		IdKey: h.Id,
	}

	change := adb.Change{
		ReturnNew: true,
		Update: bson.M{
			"$inc": bson.M{TaskCountKey: 1},
		},
	}

	info, err := db.FindAndModify(Collection, query, []string{}, change, h)
	if err != nil {
		return errors.WithStack(err)
	}

	if info.Updated != 1 {
		return errors.Errorf("expected to update one host document but actually updated %d", info.Updated)
	}

	return nil

}

func (h *Host) IncContainerBuildAttempt() error {
	query := bson.M{
		IdKey: h.Id,
	}

	change := adb.Change{
		ReturnNew: true,
		Update: bson.M{
			"$inc": bson.M{ContainerBuildAttempt: 1},
		},
	}

	info, err := db.FindAndModify(Collection, query, []string{}, change, h)
	if err != nil {
		return errors.WithStack(err)
	}

	if info.Updated != 1 {
		return errors.Errorf("expected to update one host document but actually updated %d", info.Updated)
	}

	return nil
}

func (h *Host) IncIdleTime(dur time.Duration) error {
	if dur < 0 {
		return errors.Errorf("cannot increment idle time by a negative duration %s", dur)
	}

	query := bson.M{
		IdKey: h.Id,
	}

	change := adb.Change{
		ReturnNew: true,
		Update: bson.M{
			"$inc": bson.M{TotalIdleTimeKey: dur},
		},
	}

	info, err := db.FindAndModify(Collection, query, []string{}, change, h)
	if err != nil {
		return errors.WithStack(err)
	}

	if info.Updated != 1 {
		return errors.Errorf("expected to update one host document but actually updated %d", info.Updated)
	}

	return nil
}
