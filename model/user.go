package model

import (
	"10gen.com/mci"
	"10gen.com/mci/auth"
	"10gen.com/mci/db"
	"10gen.com/mci/thirdparty"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

const (
	UsersCollection = "users"
)

type DBUser struct {
	Id           string       `bson:"_id"`
	FirstName    string       `bson:"first_name"`
	LastName     string       `bson:"last_name"`
	DispName     string       `bson:"display_name"`
	EmailAddress string       `bson:"email"`
	PubKeys      []PubKey     `bson:"public_keys" json:"public_keys"`
	CreatedAt    time.Time    `bson:"created_at"`
	Settings     UserSettings `bson:"settings"`
}

type PubKey struct {
	Name      string    `bson:"name" json:"name"`
	Key       string    `bson:"key" json:"key"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

type UserSettings struct {
	Timezone string `json:"timezone" bson:"timezone"`
}

var (
	// bson fields for the user struct
	UserIdKey           = MustHaveBsonTag(DBUser{}, "Id")
	UserFirstNameKey    = MustHaveBsonTag(DBUser{}, "FirstName")
	UserLastNameKey     = MustHaveBsonTag(DBUser{}, "LastName")
	UserDispNameKey     = MustHaveBsonTag(DBUser{}, "DispName")
	UserEmailAddressKey = MustHaveBsonTag(DBUser{}, "EmailAddress")
	UserCreatedAtKey    = MustHaveBsonTag(DBUser{}, "CreatedAt")
	UserSettingsKey     = MustHaveBsonTag(DBUser{}, "Settings")

	// bson fields for the user settings struct
	UserSettingsTZKey = MustHaveBsonTag(UserSettings{}, "Timezone")
)

func (self *DBUser) Username() string {
	return self.Id
}

func (self *DBUser) DisplayName() string {
	if self.DispName != "" {
		return self.DispName
	}
	return self.Id
}

func (self *DBUser) Email() string {
	return self.EmailAddress
}

func (self *DBUser) GetPublicKey(keyname string) (string, error) {
	for _, publicKey := range self.PubKeys {
		if publicKey.Name == keyname {
			return publicKey.Key, nil
		}
	}
	return "", fmt.Errorf("Unable to find public key '%v' for user '%v'",
		keyname, self.Username())
}

func (self *DBUser) PublicKeys() []PubKey {
	return self.PubKeys
}

func (self *DBUser) Insert() error {
	self.CreatedAt = time.Now()
	return db.Insert(UsersCollection, self)
}

// AddPublicKey adds a public key to the user's saved key list
func (self *DBUser) AddPublicKey(name, value string) error {
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	pubKey := PubKey{
		Name:      name,
		Key:       value,
		CreatedAt: time.Now(),
	}

	selector := bson.M{
		"_id": self.Id,
		"public_keys.name": bson.M{
			"$ne": pubKey.Name,
		},
	}
	update := bson.M{
		"$push": bson.M{
			"public_keys": pubKey,
		},
	}
	err = db.C(UsersCollection).Update(selector, update)
	if err == mgo.ErrNotFound {
		return fmt.Errorf("Please enter an unused public key name; '%v' is already taken", name)
	}
	return err
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
	return db.C(UsersCollection).Update(selector, update)
}

func UpdateOneUser(userId string, update interface{}) error {
	return db.Update(
		UsersCollection,
		bson.M{
			UserIdKey: userId,
		},
		update,
	)
}

func FindOneDBUser(userId string) (*DBUser, error) {
	user := &DBUser{}
	err := db.FindOne(
		UsersCollection,
		bson.M{
			UserIdKey: userId,
		},
		db.NoProjection,
		db.NoSort,
		user,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return user, err
}

type DBUserManager struct{}

func (self *DBUserManager) GetUserById(userId string) (auth.MCIUser, error) {
	user, err := FindOneDBUser(userId)
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

// Fetches a user with the given userId and returns it. If no document exists for that
// userId, inserts it along with the provided display name and email.
func GetOrCreateUser(userId, displayName, email string) (*DBUser, error) {
	u := &DBUser{}
	_, err := db.FindAndModify(UsersCollection,
		bson.M{UserIdKey: userId},
		mgo.Change{
			Update: bson.M{
				"$set": bson.M{
					UserDispNameKey:     displayName,
					UserEmailAddressKey: email,
				},
			},
			ReturnNew: true,
			Upsert:    true,
		}, u)
	if err != nil {
		return nil, err
	}
	return u, nil
}
