package db

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

const (
	GlobalsCollection = "globals"
	LastBuildId       = "last_agent_build"
)

// LastAgentBuild stores the last version of the agent that was built.
type LastAgentBuild struct {
	Id   string `bson:"_id"`
	Hash string `bson:"hash"`
}

// GetLastAgentBuild returns the most recent agent githash stored in the database.
func GetLastAgentBuild() (string, error) {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	// get the record of the last build
	lastBuild := LastAgentBuild{}
	err = db.C(GlobalsCollection).Find(bson.M{"_id": LastBuildId}).One(&lastBuild)

	if err != nil {
		if err == mgo.ErrNotFound {
			// this is fine, it just means we're definitely going to have to
			// build the agent
			return "", nil
		}
		return "", err
	}

	return lastBuild.Hash, nil
}

// StoreLastAgentBuild stores the given agent git hash in the database,
// so that Evergreen can detect when a new version of the agent should
// be compiled.
func StoreLastAgentBuild(hash string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	_, err = db.C(GlobalsCollection).Upsert(bson.M{"_id": LastBuildId},
		bson.M{"$set": bson.M{"hash": hash}})
	return err
}

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
