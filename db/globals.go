package db

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	GlobalsCollection = "globals"
)

// Global stores internal tracking information.
type Global struct {
	BuildVariant    string `bson:"_id"`
	LastBuildNumber uint64 `bson:"last_build_number"`
	LastTaskNumber  uint64 `bson:"last_task_number"`
}

// GetNewBuildVariantTaskNumber atomically gets a new number for a task,
// given its variant name.
func GetNewBuildVariantTaskNumber(buildVariant string) (uint64, error) {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return 0, err
	}
	defer session.Close()

	// get the record for this build variant
	global := Global{}
	err = db.C(GlobalsCollection).Find(bson.M{"_id": buildVariant}).One(&global)

	// if this is the first task for
	// this build variant, insert it
	if err != nil && err == mgo.ErrNotFound {
		change := mgo.Change{
			Update:    bson.M{"$inc": bson.M{"last_task_number": 1}},
			Upsert:    true,
			ReturnNew: true,
		}

		_, err = db.C(GlobalsCollection).Find(bson.M{"_id": buildVariant}).Apply(change, &global)
		if err != nil {
			return 0, err
		}
		return global.LastTaskNumber, nil
	}

	// use the current build number the first time
	// any client requests the build variant's task number
	var newBuildVariantTaskNumber uint64
	if global.LastTaskNumber == uint64(0) {
		newBuildVariantTaskNumber = global.LastBuildNumber
	} else {
		newBuildVariantTaskNumber = global.LastTaskNumber + 1
	}
	change := mgo.Change{
		Update: bson.M{"$set": bson.M{
			"last_task_number": newBuildVariantTaskNumber,
		}},
		ReturnNew: true,
	}

	_, err = db.C(GlobalsCollection).Find(bson.M{"_id": buildVariant}).Apply(change, &global)

	if err != nil {
		return 0, err
	}

	return global.LastTaskNumber, nil
}

// GetNewBuildVariantBuildNumber atomically gets a new number for a build,
// given its variant name.
func GetNewBuildVariantBuildNumber(buildVariant string) (uint64, error) {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return 0, err
	}
	defer session.Close()

	// get the record for this build variant
	global := Global{}
	err = db.C(GlobalsCollection).Find(bson.M{"_id": buildVariant}).One(&global)

	// if this is the first build for this
	// this build variant, insert it
	if err != nil && err == mgo.ErrNotFound {
		change := mgo.Change{
			Update:    bson.M{"$inc": bson.M{"last_build_number": 1}},
			Upsert:    true,
			ReturnNew: true,
		}

		_, err = db.C(GlobalsCollection).Find(bson.M{"_id": buildVariant}).Apply(change, &global)
		if err != nil {
			return 0, err
		}
		return global.LastBuildNumber, nil
	}

	// At this point, we know we've
	// seen this build variant before
	// find and modify last build variant number
	change := mgo.Change{
		Update:    bson.M{"$inc": bson.M{"last_build_number": 1}},
		ReturnNew: true,
	}

	_, err = db.C(GlobalsCollection).Find(bson.M{"_id": buildVariant}).Apply(change, &global)

	if err != nil {
		return 0, err
	}

	return global.LastBuildNumber, nil
}
