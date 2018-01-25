package db

import (
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	lockCollection = "lock"
	lockTimeout    = 8 * time.Minute
)

// Lock represents a lock stored in the database, for synchronization.
type Lock struct {
	Id       string    `bson:"_id"`
	Locked   bool      `bson:"locked"`
	LockedAt time.Time `bson:"locked_at"`
}

// WaitTillAcquireLock "spins" on acquiring the given database lock,
// for the process id, until the lock times out. Returns whether or
// not the lock was acquired.
func WaitTillAcquireLock(id string) (bool, error) {
	startTime := time.Now()
	for {
		// if the timeout has been reached, we failed to get the lock
		currTime := time.Now()
		if startTime.Add(lockTimeout).Before(currTime) {
			return false, nil
		}

		// attempt to get the lock
		acquired, err := AcquireLock(id)
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
		}

		// sleep
		time.Sleep(time.Second)
	}
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
	timeoutThreshold := time.Now().Add(-lockTimeout)

	// construct the selector for the following cases:
	// 1. lock is not held by anyone
	// 2. lock is held but has timed out
	selector := bson.M{
		"_id": id,
		"$or": []bson.M{
			{"locked": false},
			{"locked_at": bson.M{"$lte": timeoutThreshold}},
		},
	}

	// change to apply to document
	change := mgo.Change{
		Update: bson.M{"$set": bson.M{
			"locked":    true,
			"locked_at": time.Now(),
		}},
		Upsert:    upsert,
		ReturnNew: true,
	}

	lock := Lock{}

	// gets the lock if we can
	_, err = db.C(lockCollection).Find(selector).Apply(change, &lock)

	if err != nil {
		return false, err
	}
	return lock.Locked, nil
}

// AcquireLock attempts to acquire the specified lock if
// no one has it or it's timed out. Returns a boolean indicating
// whether the lock was acquired.
func AcquireLock(id string) (bool, error) {
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

// ReleaseLock relinquishes the lock for the given id.
func ReleaseLock(id string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// will return mgo.ErrNotFound if the lock expired
	return db.C(lockCollection).Update(
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"locked": false}},
	)
}
