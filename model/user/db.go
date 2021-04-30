package user

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	Collection = "users"
)

var (
	IdKey                     = bsonutil.MustHaveTag(DBUser{}, "Id")
	FirstNameKey              = bsonutil.MustHaveTag(DBUser{}, "FirstName")
	LastNameKey               = bsonutil.MustHaveTag(DBUser{}, "LastName")
	DispNameKey               = bsonutil.MustHaveTag(DBUser{}, "DispName")
	EmailAddressKey           = bsonutil.MustHaveTag(DBUser{}, "EmailAddress")
	PatchNumberKey            = bsonutil.MustHaveTag(DBUser{}, "PatchNumber")
	CreatedAtKey              = bsonutil.MustHaveTag(DBUser{}, "CreatedAt")
	SettingsKey               = bsonutil.MustHaveTag(DBUser{}, "Settings")
	APIKeyKey                 = bsonutil.MustHaveTag(DBUser{}, "APIKey")
	OnlyAPIKey                = bsonutil.MustHaveTag(DBUser{}, "OnlyAPI")
	PubKeysKey                = bsonutil.MustHaveTag(DBUser{}, "PubKeys")
	LoginCacheKey             = bsonutil.MustHaveTag(DBUser{}, "LoginCache")
	RolesKey                  = bsonutil.MustHaveTag(DBUser{}, "SystemRoles")
	LoginCacheTokenKey        = bsonutil.MustHaveTag(LoginCache{}, "Token")
	LoginCacheTTLKey          = bsonutil.MustHaveTag(LoginCache{}, "TTL")
	LoginCacheAccessTokenKey  = bsonutil.MustHaveTag(LoginCache{}, "AccessToken")
	LoginCacheRefreshTokenKey = bsonutil.MustHaveTag(LoginCache{}, "RefreshToken")
	PubKeyNameKey             = bsonutil.MustHaveTag(PubKey{}, "Name")
	PubKeyKey                 = bsonutil.MustHaveTag(PubKey{}, "Key")
	PubKeyNCreatedAtKey       = bsonutil.MustHaveTag(PubKey{}, "CreatedAt")
	FavoriteProjectsKey       = bsonutil.MustHaveTag(DBUser{}, "FavoriteProjects")
)

//nolint: deadcode, megacheck, unused
var (
	githubUserUID         = bsonutil.MustHaveTag(GithubUser{}, "UID")
	githubUserLastKnownAs = bsonutil.MustHaveTag(GithubUser{}, "LastKnownAs")
)

var (
	SettingsTZKey             = bsonutil.MustHaveTag(UserSettings{}, "Timezone")
	userSettingsGithubUserKey = bsonutil.MustHaveTag(UserSettings{}, "GithubUser")
	UseSpruceOptionsKey       = bsonutil.MustHaveTag(UserSettings{}, "UseSpruceOptions")
	SpruceV1Key               = bsonutil.MustHaveTag(UseSpruceOptions{}, "SpruceV1")
)

// ById returns a query that matches a user by ID.
func ById(userId string) db.Q {
	return db.Query(bson.M{IdKey: userId})
}

// ByIds returns a query that matches any users with one of the given IDs.
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

// FindByGithubUID finds a user with the given GitHub UID.
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

// FindByGithubName finds a user with the given GitHub username.
func FindByGithubName(name string) (*DBUser, error) {
	u := DBUser{}
	err := db.FindOneQ(Collection, db.Query(bson.M{
		bsonutil.GetDottedKeyName(SettingsKey, userSettingsGithubUserKey, githubUserLastKnownAs): name,
	}), &u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch user by github name")
	}

	return &u, nil
}

// GetPatchUser gets a user from their GitHub UID. If no such user is found, it
// defaults to the global GitHub pull request user.
func GetPatchUser(gitHubUID int) (*DBUser, error) {
	u, err := FindByGithubUID(gitHubUID)
	if err != nil {
		return nil, errors.Wrap(err, "can't look for user")
	}
	if u == nil {
		// set to a default user
		u, err = FindOne(ById(evergreen.GithubPatchUser))
		if err != nil {
			return nil, errors.Wrap(err, "can't get user for pull request")
		}
		// default user doesn't exist yet
		if u == nil {
			u = &DBUser{
				Id:       evergreen.GithubPatchUser,
				DispName: "Github Pull Requests",
				APIKey:   utility.RandomString(),
			}
			if err = u.Insert(); err != nil {
				return nil, errors.Wrap(err, "failed to create github patch user")
			}
		}
	}

	return u, nil
}

// AddOrUpdateServiceUser upserts a service user by ID. If it's a new user, it
// generates a new API key for the user.
func AddOrUpdateServiceUser(u DBUser) error {
	if !u.OnlyAPI {
		return errors.New("cannot update a non-service user")
	}
	query := bson.M{
		IdKey: u.Id,
	}
	apiKey := u.APIKey
	if apiKey == "" {
		apiKey = utility.RandomString()
	}
	update := bson.M{
		"$set": bson.M{
			DispNameKey: u.DispName,
			RolesKey:    u.SystemRoles,
			OnlyAPIKey:  true,
			APIKeyKey:   apiKey,
		},
	}
	_, err := UpsertOne(query, update)
	return err
}

// DeleteServiceUser deletes a service user by ID.
func DeleteServiceUser(id string) error {
	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()
	query := bson.M{
		IdKey:      id,
		OnlyAPIKey: true,
	}
	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	result, err := coll.DeleteOne(ctx, query)
	if err != nil {
		return errors.Wrap(err, "unable to delete service user")
	}
	if result.DeletedCount < 1 {
		return errors.Errorf("service user '%s' not found", id)
	}
	return nil
}

// GetOrCreateUser upserts a user with the given userId with the given display
// name, email, access token, and refresh token and returns the updated user. If
// no such user exists for that userId yet, it also sets the user's API key and
// roles.
func GetOrCreateUser(userId, displayName, email, accessToken, refreshToken string, roles []string) (*DBUser, error) {
	u := &DBUser{}
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	setFields := bson.M{
		DispNameKey:     displayName,
		EmailAddressKey: email,
	}
	if accessToken != "" {
		setFields[bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheAccessTokenKey)] = accessToken
	}
	if refreshToken != "" {
		setFields[bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheRefreshTokenKey)] = refreshToken
	}
	setOnInsertFields := bson.M{
		APIKeyKey: utility.RandomString(),
		bsonutil.GetDottedKeyName(SettingsKey, UseSpruceOptionsKey, SpruceV1Key): true,
	}
	if roles != nil && len(roles) > 0 {
		setOnInsertFields[RolesKey] = roles
	}
	res := env.DB().Collection(Collection).FindOneAndUpdate(ctx,
		bson.M{IdKey: userId},
		bson.M{
			"$set":         setFields,
			"$setOnInsert": setOnInsertFields,
		},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	)

	if err := res.Err(); err != nil {
		return nil, errors.Wrapf(err, "problem find/create user '%s'", userId)
	}

	if err := res.Decode(u); err != nil {
		return nil, errors.Wrapf(err, "problem decoding result for user '%s'", userId)

	}
	return u, nil
}

// FindNeedsReauthorization finds all users that need to be reauthorized after
// the given period has passed and who have not exceeded the max reauthorization
// attempts.
func FindNeedsReauthorization(reauthorizeAfter time.Duration) ([]DBUser, error) {
	cutoff := time.Now().Add(-reauthorizeAfter)
	users, err := Find(db.Query(bson.M{
		bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTokenKey): bson.M{"$exists": true},
		bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTTLKey):   bson.M{"$lte": cutoff},
	}))
	return users, errors.Wrap(err, "could not find users who need reauthorization")
}

// FindServiceUsers returns all API-only users.
func FindServiceUsers() ([]DBUser, error) {
	query := bson.M{
		OnlyAPIKey: true,
	}
	return Find(db.Query(query))
}

// PutLoginCache generates a token if one does not exist, and sets the TTL to
// now.
func PutLoginCache(g gimlet.User) (string, error) {
	u, err := FindOneById(g.Username())
	if err != nil {
		return "", errors.Wrap(err, "problem finding user by id")
	}
	if u == nil {
		return "", errors.Errorf("no user '%s' found", g.Username())
	}

	// Always update the TTL. If the user doesn't have a token, generate and set it.
	token := u.LoginCache.Token
	setFields := bson.M{
		bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTTLKey): time.Now(),
	}
	if token == "" {
		token = utility.RandomString()
		setFields[bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTokenKey)] = token
	}
	if accessToken := g.GetAccessToken(); accessToken != "" {
		setFields[bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheAccessTokenKey)] = accessToken
	}
	if refreshToken := g.GetRefreshToken(); refreshToken != "" {
		setFields[bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheRefreshTokenKey)] = refreshToken
	}
	update := bson.M{"$set": setFields}

	if err := UpdateOne(bson.M{IdKey: u.Id}, update); err != nil {
		return "", errors.Wrap(err, "problem updating user cache")
	}
	return token, nil
}

// GetLoginCache retrieve a cached user by token.
// It returns an error if and only if there was an error retrieving the user from the cache.
// It returns (<user>, true, nil) if the user is present in the cache and is valid.
// It returns (<user>, false, nil) if the user is present in the cache but has expired.
// It returns (nil, false, nil) if the user is not present in the cache.
func GetLoginCache(token string, expireAfter time.Duration) (gimlet.User, bool, error) {
	u, err := FindOneByToken(token)
	if err != nil {
		return nil, false, errors.Wrap(err, "problem getting user from cache")
	}
	if u == nil {
		return nil, false, nil
	}
	if time.Since(u.LoginCache.TTL) > expireAfter {
		return u, false, nil
	}
	return u, true, nil
}

// ClearLoginCache clears one user's login cache, forcibly logging them out.
func ClearLoginCache(user gimlet.User) error {
	update := bson.M{"$unset": bson.M{LoginCacheKey: 1}}

	u, err := FindOneById(user.Username())
	if err != nil {
		return errors.Wrapf(err, "problem finding user %s by id", user.Username())
	}
	if u == nil {
		return errors.Errorf("no user '%s' found", user.Username())
	}
	query := bson.M{IdKey: u.Id}
	if err := UpdateOne(query, update); err != nil {
		return errors.Wrap(err, "problem updating user cache")
	}

	return nil
}

// ClearAllLoginCaches clears all users' login caches, forcibly logging them
// out.
func ClearAllLoginCaches() error {
	update := bson.M{"$unset": bson.M{LoginCacheKey: 1}}
	query := bson.M{}
	if err := UpdateAll(query, update); err != nil {
		return errors.Wrap(err, "problem updating user cache")
	}
	return nil
}
