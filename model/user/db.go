package user

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

const (
	Collection = "users"
)

var (
	// bson fields for the user struct
	IdKey           = bsonutil.MustHaveTag(DBUser{}, "Id")
	FirstNameKey    = bsonutil.MustHaveTag(DBUser{}, "FirstName")
	LastNameKey     = bsonutil.MustHaveTag(DBUser{}, "LastName")
	DispNameKey     = bsonutil.MustHaveTag(DBUser{}, "DispName")
	EmailAddressKey = bsonutil.MustHaveTag(DBUser{}, "EmailAddress")
	CreatedAtKey    = bsonutil.MustHaveTag(DBUser{}, "CreatedAt")
	SettingsKey     = bsonutil.MustHaveTag(DBUser{}, "Settings")
	APIKeyKey       = bsonutil.MustHaveTag(DBUser{}, "APIKey")
	PubKeysKey      = bsonutil.MustHaveTag(DBUser{}, "PubKeys")
	PubKeyNameKey   = bsonutil.MustHaveTag(PubKey{}, "Name")

	// bson fields for the user settings struct
	SettingsTZKey = bsonutil.MustHaveTag(UserSettings{}, "Timezone")
)

func ById(userId string) db.Q {
	return db.Query(bson.M{IdKey: userId})
}

func ByIds(userIds ...string) db.Q {
	return db.Query(bson.M{IdKey: bson.M{"$in": userIds}})
}

// Not yet implemented in the UI
func (self *DBUser) RemovePublicKey(name string) error {
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	selector := bson.M{
		"_id": self.Id,
	}
	update := bson.M{
		"$pull": bson.M{
			"public_keys.name": name,
		},
	}
	return db.C(Collection).Update(selector, update)
}

// FindOne gets one DBUser for the given query.
func FindOne(query db.Q) (*DBUser, error) {
	u := &DBUser{}
	err := db.FindOneQ(Collection, query, u)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return u, err
}

// Find gets all DBUser for the given query.
func Find(query db.Q) ([]DBUser, error) {
	us := []DBUser{}
	err := db.FindAllQ(Collection, query, &us)
	return us, err
}

// Count returns the number of user that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}

// UpdateOne updates one user.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

// UpdateAll updates all users.
func UpdateAll(query interface{}, update interface{}) error {
	_, err := db.UpdateAll(
		Collection,
		query,
		update,
	)
	return err
}

// UpsertOne upserts a user.
func UpsertOne(query interface{}, update interface{}) (*mgo.ChangeInfo, error) {
	return db.Upsert(
		Collection,
		query,
		update,
	)
}
