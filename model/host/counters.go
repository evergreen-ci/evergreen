package host

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (h *Host) IncProvisionAttempts() error {
	query := bson.M{
		IdKey: h.Id,
	}

	change := mgo.Change{
		ReturnNew: true,
		Update: bson.M{
			"$inc": bson.M{ProvisionAttemptsKey: 1},
		},
	}

	info, err := db.FindAndModify(Collection, query, []string{}, change, h)
	if err != nil {
		return errors.WithStack(err)
	}

	if info.Updated != 1 {
		return errors.Errorf("could not find host document to update, %s", h.Id)
	}

	return nil
}

func (h *Host) IncTaskCount() error {
	query := bson.M{
		IdKey: h.Id,
	}

	change := mgo.Change{
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
		return errors.Errorf("could not find host document to update, %s", h.Id)
	}

	return nil

}

func (h *Host) IncIdleTime(dur time.Duration) error {
	if dur < 0 {
		return errors.Errorf("cannot increment by a negative duration value [%s]", dur)
	}

	query := bson.M{
		IdKey: h.Id,
	}

	change := mgo.Change{
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
		return errors.Errorf("could not find host document to update, %s", h.Id)
	}

	return nil
}

func (h *Host) IncCost(amt float64) error {
	if amt < 0 {
		return errors.Errorf("cost must be a positive value [%g]", amt)
	}

	query := bson.M{IdKey: h.Id}

	change := mgo.Change{
		ReturnNew: true,
		Update: bson.M{
			"$inc": bson.M{TotalCostKey: amt},
		},
	}

	info, err := db.FindAndModify(Collection, query, []string{}, change, h)
	if err != nil {
		return errors.WithStack(err)
	}

	if info.Updated != 1 {
		return errors.Errorf("could not find host document to update %s", h.Id)
	}

	return nil
}
