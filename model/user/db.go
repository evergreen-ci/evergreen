package user

import (
	"10gen.com/mci"
	"10gen.com/mci/auth"
	"10gen.com/mci/db"
	"10gen.com/mci/db/bsonutil"
	"10gen.com/mci/thirdparty"
	"github.com/10gen-labs/slogger/v1"
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

type DBUserManager struct{}

func (self *DBUserManager) GetUserById(userId string) (auth.MCIUser, error) {
	user, err := FindOne(ById(userId))
	if err != nil {
		return nil, err
	} else if user == nil {
		return nil, nil
	} else {
		return user, nil
	}
}

//DBCachedCrowdUserManager is a usermanager that looks up users in the database
//and fetches them from crowd if they are not there (and creates DB record)
type DBCachedCrowdUserManager struct {
	thirdparty.RESTCrowdService
	DBUserManager
}

func (umgr *DBCachedCrowdUserManager) GetUserSession(username string,
	password string) (*thirdparty.Session, error) {
	return umgr.RESTCrowdService.CreateSession(username, password)
}

func (umgr *DBCachedCrowdUserManager) GetUserById(token string) (auth.MCIUser,
	error) {

	crowdUser, err := umgr.RESTCrowdService.GetUserFromToken(token)
	if err != nil {
		return nil, err
	}

	// Crowd user is valid. See if they exist in DB
	user, err := umgr.DBUserManager.GetUserById(crowdUser.Name)
	if err != nil {
		mci.Logger.Logf(slogger.ERROR, "Error getting user obj from db: %v",
			err)
		return nil, err
	}
	if user != nil {
		return user, nil
	}

	// User doesn't exist in DB. Create them and save them.
	newUser := &DBUser{
		Id:           crowdUser.Name,
		FirstName:    crowdUser.FirstName,
		LastName:     crowdUser.LastName,
		DispName:     crowdUser.DispName,
		EmailAddress: crowdUser.EmailAddress,
	}
	if err = newUser.Insert(); err != nil {
		mci.Logger.Logf(slogger.ERROR, "Error inserting user obj: %v", err)
		return nil, err
	}
	return newUser, nil
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
