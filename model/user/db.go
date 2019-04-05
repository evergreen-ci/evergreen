package user

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	Collection = "users"
)

var (
	IdKey               = bsonutil.MustHaveTag(DBUser{}, "Id")
	FirstNameKey        = bsonutil.MustHaveTag(DBUser{}, "FirstName")
	LastNameKey         = bsonutil.MustHaveTag(DBUser{}, "LastName")
	DispNameKey         = bsonutil.MustHaveTag(DBUser{}, "DispName")
	EmailAddressKey     = bsonutil.MustHaveTag(DBUser{}, "EmailAddress")
	PatchNumberKey      = bsonutil.MustHaveTag(DBUser{}, "PatchNumber")
	CreatedAtKey        = bsonutil.MustHaveTag(DBUser{}, "CreatedAt")
	SettingsKey         = bsonutil.MustHaveTag(DBUser{}, "Settings")
	APIKeyKey           = bsonutil.MustHaveTag(DBUser{}, "APIKey")
	PubKeysKey          = bsonutil.MustHaveTag(DBUser{}, "PubKeys")
	LoginCacheKey       = bsonutil.MustHaveTag(DBUser{}, "LoginCache")
	LoginCacheTokenKey  = bsonutil.MustHaveTag(LoginCache{}, "Token")
	LoginCacheTTLKey    = bsonutil.MustHaveTag(LoginCache{}, "TTL")
	PubKeyNameKey       = bsonutil.MustHaveTag(PubKey{}, "Name")
	PubKeyKey           = bsonutil.MustHaveTag(PubKey{}, "Key")
	PubKeyNCreatedAtKey = bsonutil.MustHaveTag(PubKey{}, "CreatedAt")
)

//nolint: deadcode, megacheck, unused
var (
	githubUserUID         = bsonutil.MustHaveTag(GithubUser{}, "UID")
	githubUserLastKnownAs = bsonutil.MustHaveTag(GithubUser{}, "LastKnownAs")
)

var (
	SettingsTZKey             = bsonutil.MustHaveTag(UserSettings{}, "Timezone")
	userSettingsGithubUserKey = bsonutil.MustHaveTag(UserSettings{}, "GithubUser")
)

func FindByGithubUID(uid int) (*DBUser, error) {
	u := DBUser{}
	err := db.FindOneQ(Collection, db.Query(bson.M{
		bsonutil.GetDottedKeyName(SettingsKey, userSettingsGithubUserKey, githubUserUID): uid,
	}), &u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch user by github uid")
	}

	return &u, nil
}

func ById(userId string) db.Q {
	return db.Query(bson.M{IdKey: userId})
}

func ByIds(userIds ...string) db.Q {
	return db.Query(bson.M{
		IdKey: bson.M{
			"$in": userIds,
		},
	})
}

// FindOne gets one DBUser for the given query.
func FindOne(query db.Q) (*DBUser, error) {
	u := &DBUser{}
	err := db.FindOneQ(Collection, query, u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return u, err
}

// FindOneByToken gets a DBUser by cached login token.
func FindOneByToken(token string) (*DBUser, error) {
	u := &DBUser{}
	query := db.Query(bson.M{bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTokenKey): token})
	err := db.FindOneQ(Collection, query, u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem finding user by token")
	}
	return u, nil
}

// FindOneById gets a DBUser by ID.
func FindOneById(id string) (*DBUser, error) {
	u := &DBUser{}
	query := ById(id)
	err := db.FindOneQ(Collection, query, u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem finding user by id")
	}
	return u, nil
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
func UpsertOne(query interface{}, update interface{}) (*adb.ChangeInfo, error) {
	return db.Upsert(
		Collection,
		query,
		update,
	)
}
