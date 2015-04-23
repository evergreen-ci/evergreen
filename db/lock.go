package db

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

var _ fmt.Stringer = nil

const (
	LockCollection = "lock"
	GlobalLockId   = "global"
	LockTimeout    = time.Millisecond * 3 * 60000
)

type Lock struct {
	Id       string    `bson:"_id"`
	Locked   bool      `bson:"locked"`
	LockedBy string    `bson:"locked_by"`
	LockedAt time.Time `bson:"locked_at"`
}

// should be called once, at program initialization,
// maybe by the repotracker (since it is the first to run of the cron job)
func InitializeGlobalLock() error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// for safety's sake, check if it's there.  this will make this
	// function idempotent
	lock := Lock{}
	err = db.C(LockCollection).Find(bson.M{"_id": GlobalLockId}).One(&lock)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}

	// already exists
	if lock.Id != "" {
		return nil
	}

	return db.C(LockCollection).Insert(bson.M{"_id": GlobalLockId, "locked": false})
}

func WaitTillAcquireGlobalLock(id string, timeoutMS time.Duration) (bool, error) {
	startTime := time.Now()
	for {
		// if the timeout has been reached, we failed to get the lock
		currTime := time.Now()
		if startTime.Add(timeoutMS * time.Millisecond).Before(currTime) {
			return false, nil
		}

		// attempt to get the lock
		acquired, err := AcquireGlobalLock(id)
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
		}

		// sleep
		time.Sleep(1000 * time.Millisecond)
	}

	// won't get here, but go forces it
	return false, nil
}

// attempt to acquire the global lock of no one has it
func setDocumentLocked(id string, upsert bool) (bool, error) {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return false, err
	}
	defer session.Close()

	// for findAndModify-ing the lock

	// timeout to check for
	timeoutThreshold := time.Now().Add(-LockTimeout)

	// construct the selector for the following cases:
	// 1. lock is not held by anyone
	// 2. lock is held but has timed out
	selector := bson.M{
		"_id": GlobalLockId,
		"$or": []bson.M{bson.M{"locked": false}, bson.M{"locked_at": bson.M{"$lte": timeoutThreshold}}},
	}

	// change to apply to document
	change := mgo.Change{
		Update: bson.M{"$set": bson.M{
			"locked":    true,
			"locked_by": id,
			"locked_at": time.Now(),
		}},
		Upsert:    upsert,
		ReturnNew: true,
	}

	lock := Lock{}

	// gets the lock if we can
	_, err = db.C(LockCollection).Find(selector).Apply(change, &lock)

	if err != nil {
		return false, err
	}
	return lock.Locked, nil
}

// attempt to acquire the global lock if no one has it or it's timed out
func AcquireGlobalLock(id string) (bool, error) {
	acquired, err := setDocumentLocked(id, false)

	if err == mgo.ErrNotFound {
		// in the case where no lock document exists
		// this will return a duplicate key error if
		// another lock contender grabs the lock before
		// we are able to
		acquired, err = setDocumentLocked(id, true)

		// since we're upserting now, don't
		// return any duplicate key errors
		if mgo.IsDup(err) {
			return acquired, nil
		}
		return acquired, err
	}
	return acquired, err
}

func ReleaseGlobalLock(id string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// will return mgo.ErrNotFound if the lock expired
	return db.C(LockCollection).Update(bson.M{"_id": GlobalLockId, "locked_by": id}, bson.M{"$set": bson.M{"locked": false}})
}
