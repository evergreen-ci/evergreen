package user

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	LastScheduledTasksAtKey   = bsonutil.MustHaveTag(DBUser{}, "LastScheduledTasksAt")
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
	ParsleyFiltersKey         = bsonutil.MustHaveTag(DBUser{}, "ParsleyFilters")
	ParsleySettingsKey        = bsonutil.MustHaveTag(DBUser{}, "ParsleySettings")
	NumScheduledPatchTasksKey = bsonutil.MustHaveTag(DBUser{}, "NumScheduledPatchTasks")
	BetaFeaturesKey           = bsonutil.MustHaveTag(DBUser{}, "BetaFeatures")
)

var (
	GithubUserUIDKey         = bsonutil.MustHaveTag(GithubUser{}, "UID")
	githubUserLastKnownAsKey = bsonutil.MustHaveTag(GithubUser{}, "LastKnownAs")
)

var (
	SettingsTZKey                = bsonutil.MustHaveTag(UserSettings{}, "Timezone")
	UserSettingsGithubUserKey    = bsonutil.MustHaveTag(UserSettings{}, "GithubUser")
	userSettingsSlackUsernameKey = bsonutil.MustHaveTag(UserSettings{}, "SlackUsername")
	userSettingsSlackMemberIdKey = bsonutil.MustHaveTag(UserSettings{}, "SlackMemberId")
	UseSpruceOptionsKey          = bsonutil.MustHaveTag(UserSettings{}, "UseSpruceOptions")
	SpruceV1Key                  = bsonutil.MustHaveTag(UseSpruceOptions{}, "SpruceV1")
)

// ById returns a query that matches a user by ID.
func ById(userId string) db.Q {
	return db.Query(bson.M{IdKey: userId})
}

// FindOne gets one DBUser for the given query.
func FindOne(ctx context.Context, query db.Q) (*DBUser, error) {
	u := &DBUser{}
	err := db.FindOneQContext(ctx, Collection, query, u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return u, err
}

// FindOneById gets a DBUser by ID.
func FindOneById(ctx context.Context, id string) (*DBUser, error) {
	u := &DBUser{}
	query := ById(id)
	err := db.FindOneQContext(ctx, Collection, query, u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding user by ID")
	}
	return u, nil
}

// Find gets all DBUser for the given query.
func Find(ctx context.Context, query db.Q) ([]DBUser, error) {
	us := []DBUser{}
	err := db.FindAllQ(ctx, Collection, query, &us)
	return us, err
}

// UpdateOne updates one user.
func UpdateOne(ctx context.Context, query any, update any) error {
	return db.Update(
		ctx,
		Collection,
		query,
		update,
	)
}

// UpdateAll updates all users.
func UpdateAll(ctx context.Context, query any, update any) error {
	_, err := db.UpdateAll(
		ctx,
		Collection,
		query,
		update,
	)
	return err
}

// UpsertOne upserts a user.
func UpsertOne(ctx context.Context, query any, update any) (*adb.ChangeInfo, error) {
	return db.Upsert(
		ctx,
		Collection,
		query,
		update,
	)
}

// FindOneByToken gets a DBUser by cached login token.
func FindOneByToken(ctx context.Context, token string) (*DBUser, error) {
	u := &DBUser{}
	query := db.Query(bson.M{bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTokenKey): token})
	err := db.FindOneQContext(ctx, Collection, query, u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding user by token")
	}
	return u, nil
}

// FindByGithubUID finds a user with the given GitHub UID.
func FindByGithubUID(ctx context.Context, uid int) (*DBUser, error) {
	u := DBUser{}
	err := db.FindOneQContext(ctx, Collection, db.Query(bson.M{
		bsonutil.GetDottedKeyName(SettingsKey, UserSettingsGithubUserKey, GithubUserUIDKey): uid,
	}), &u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding user by GitHub UID")
	}

	return &u, nil
}

// FindByGithubName finds a user with the given GitHub username.
func FindByGithubName(ctx context.Context, name string) (*DBUser, error) {
	u := DBUser{}
	err := db.FindOneQContext(ctx, Collection, db.Query(bson.M{
		bsonutil.GetDottedKeyName(SettingsKey, UserSettingsGithubUserKey, githubUserLastKnownAsKey): name,
	}), &u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding user by GitHub name")
	}

	return &u, nil
}

// FindBySlackUsername finds a user with the given Slack Username.
func FindBySlackUsername(ctx context.Context, userName string) (*DBUser, error) {
	u := DBUser{}
	err := db.FindOneQContext(ctx, Collection, db.Query(bson.M{
		bsonutil.GetDottedKeyName(SettingsKey, userSettingsSlackUsernameKey): userName,
	}), &u)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding user by Slack Username")
	}

	return &u, nil
}

func FindByRole(ctx context.Context, role string) ([]DBUser, error) {
	res := []DBUser{}
	err := db.FindAllQ(ctx,
		Collection,
		db.Query(bson.M{RolesKey: role}),
		&res,
	)
	return res, errors.Wrapf(err, "finding users with role '%s'", role)
}

// AddOrUpdateServiceUser upserts a service user by ID. If it's a new user, it
// generates a new API key for the user.
func AddOrUpdateServiceUser(ctx context.Context, u DBUser) error {
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
			DispNameKey:     u.DispName,
			RolesKey:        u.SystemRoles,
			OnlyAPIKey:      true,
			APIKeyKey:       apiKey,
			EmailAddressKey: u.EmailAddress,
		},
	}
	_, err := UpsertOne(ctx, query, update)
	return err
}

// FindHumanUsersByRoles returns human users that have any of the given roles.
func FindHumanUsersByRoles(ctx context.Context, roles []string) ([]DBUser, error) {
	res := []DBUser{}
	err := db.FindAllQ(ctx,
		Collection,
		db.Query(bson.M{
			RolesKey:   bson.M{"$in": roles},
			OnlyAPIKey: bson.M{"$ne": true},
		}),
		&res,
	)
	return res, errors.Wrapf(err, "finding users with roles '%s'", roles)
}

// GetPeriodicBuild returns the matching user if applicable, and otherwise returns the default periodic build user.
func GetPeriodicBuildUser(ctx context.Context, user string) (*DBUser, error) {
	if user != "" {
		usr, err := FindOneById(ctx, user)
		if err != nil {
			return nil, errors.Wrapf(err, "finding user '%s'", user)
		}
		return usr, nil
	}

	usr, err := FindOneById(ctx, evergreen.PeriodicBuildUser)
	if err != nil {
		return nil, errors.Wrap(err, "getting periodic build user")
	}
	return usr, nil
}

// DeleteServiceUser deletes a service user by ID.
func DeleteServiceUser(ctx context.Context, id string) error {
	query := bson.M{
		IdKey:      id,
		OnlyAPIKey: true,
	}
	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	result, err := coll.DeleteOne(ctx, query)
	if err != nil {
		return errors.Wrap(err, "deleting service user")
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
func GetOrCreateUser(ctx context.Context, userId, displayName, email, accessToken, refreshToken string, roles []string) (*DBUser, error) {
	u := &DBUser{}
	env := evergreen.GetEnvironment()
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
	if len(roles) > 0 {
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
		return nil, errors.Wrapf(err, "finding/creating user '%s'", userId)
	}

	if err := res.Decode(u); err != nil {
		return nil, errors.Wrapf(err, "decoding user '%s'", userId)

	}

	if err := setSlackInformation(ctx, env, u); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "could not set Slack information for user",
			"user_id":       u.Id,
			"email_address": u.EmailAddress,
		}))
		return u, nil
	}

	return u, nil
}

func setSlackInformation(ctx context.Context, env evergreen.Environment, u *DBUser) error {
	if isSpiffeMonitoringUser(u.Id) {
		// don't set slack information for spiffe monitoring users
		return nil
	}
	if u.Settings.SlackMemberId != "" {
		// user already has a slack member id set
		return nil
	}
	if u.EmailAddress == "" {
		// we can't fetch the slack information without the user's email address
		return errors.New("user has no email address")
	}

	slackEnv := env.Settings().Slack
	slackUser, err := slackEnv.Options.GetSlackUser(slackEnv.Token, u.EmailAddress)
	if err != nil {
		return errors.Wrapf(err, "getting Slack user with email address '%s'", u.EmailAddress)
	}
	if slackUser == nil {
		grip.Error(message.Fields{
			"message":       "Couldn't find slack user by email address",
			"user_id":       u.Id,
			"email_address": u.EmailAddress,
		})
		return nil
	}

	slackFields := bson.M{
		bsonutil.GetDottedKeyName(SettingsKey, userSettingsSlackMemberIdKey): slackUser.ID,
	}
	if slackUser.Name != "" {
		slackFields[bsonutil.GetDottedKeyName(SettingsKey, userSettingsSlackUsernameKey)] = slackUser.Name
	}

	update := bson.M{"$set": slackFields}

	if err := UpdateOne(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return errors.Wrap(err, "updating slack information")
	}

	return nil

}

const spiffeMonitoringRoute = "spiffe://cluster.local/ns/monitoring"

// isSpiffeServiceUser checks if the user is a spiffe monitoring user by checking
// if it starts with spiffe://cluster.local/ns/monitoring
func isSpiffeMonitoringUser(userId string) bool {
	return strings.HasPrefix(userId, spiffeMonitoringRoute)
}

// FindNeedsReauthorization finds all users that need to be reauthorized after
// the given period has passed and who have not exceeded the max reauthorization
// attempts.
func FindNeedsReauthorization(ctx context.Context, reauthorizeAfter time.Duration) ([]DBUser, error) {
	cutoff := time.Now().Add(-reauthorizeAfter)
	users, err := Find(ctx, db.Query(bson.M{
		bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTokenKey): bson.M{"$exists": true},
		bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTTLKey):   bson.M{"$lte": cutoff},
	}))
	return users, errors.Wrap(err, "finding users who need reauthorization")
}

// FindServiceUsers returns all API-only users.
func FindServiceUsers(ctx context.Context) ([]DBUser, error) {
	query := bson.M{
		OnlyAPIKey: true,
	}
	return Find(ctx, db.Query(query))
}

// PutLoginCache generates a token if one does not exist, and sets the TTL to
// now.
func PutLoginCache(ctx context.Context, g gimlet.User) (string, error) {
	u, err := FindOneById(ctx, g.Username())
	if err != nil {
		return "", errors.Wrap(err, "finding user by ID")
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

	if err := UpdateOne(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return "", errors.Wrap(err, "updating user cache")
	}
	return token, nil
}

// GetLoginCache retrieve a cached user by token.
// It returns an error if and only if there was an error retrieving the user from the cache.
// It returns (<user>, true, nil) if the user is present in the cache and is valid.
// It returns (<user>, false, nil) if the user is present in the cache but has expired.
// It returns (nil, false, nil) if the user is not present in the cache.
func GetLoginCache(ctx context.Context, token string, expireAfter time.Duration) (gimlet.User, bool, error) {
	u, err := FindOneByToken(ctx, token)
	if err != nil {
		return nil, false, errors.Wrap(err, "getting user from cache")
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
func ClearLoginCache(ctx context.Context, user gimlet.User) error {
	update := bson.M{"$unset": bson.M{LoginCacheKey: 1}}

	u, err := FindOneById(ctx, user.Username())
	if err != nil {
		return errors.Wrapf(err, "finding user '%s' by ID", user.Username())
	}
	if u == nil {
		return errors.Errorf("user '%s' not found", user.Username())
	}
	query := bson.M{IdKey: u.Id}
	if err := UpdateOne(ctx, query, update); err != nil {
		return errors.Wrap(err, "updating user cache")
	}

	return nil
}

// ClearUser clears the users settings, roles and invalidates their login cache.
// It also sets their settings to use Spruce so rehires have Spruce enabled by default.
func ClearUser(ctx context.Context, userId string) error {
	unsetUpdate := bson.M{
		"$unset": bson.M{
			SettingsKey:   1,
			RolesKey:      1,
			LoginCacheKey: 1,
			PubKeysKey:    1,
			APIKeyKey:     1,
		},
	}
	query := bson.M{IdKey: userId}
	if err := UpdateOne(ctx, query, unsetUpdate); err != nil {
		return errors.Wrap(err, "unsetting user settings")
	}
	setUpdate := bson.M{
		"$set": bson.M{
			SettingsKey: bson.M{
				UseSpruceOptionsKey: bson.M{
					SpruceV1Key: true,
				},
			},
		},
	}
	return errors.Wrap(UpdateOne(ctx, query, setUpdate), "defaulting spruce setting")
}

// ClearAllLoginCaches clears all users' login caches, forcibly logging them
// out.
func ClearAllLoginCaches(ctx context.Context) error {
	update := bson.M{"$unset": bson.M{LoginCacheKey: 1}}
	query := bson.M{}
	if err := UpdateAll(ctx, query, update); err != nil {
		return errors.Wrap(err, "updating user cache")
	}
	return nil
}

// UpsertOneFromExisting creates a new user with the same necessary data as oldUsr.
func UpsertOneFromExisting(ctx context.Context, oldUsr *DBUser, newEmail string) (*DBUser, error) {
	splitString := strings.Split(newEmail, "@")
	if len(splitString) == 1 {
		return nil, errors.New("email address is missing '@'")
	}
	newUsername := splitString[0]
	if newUsername == "" {
		return nil, errors.New("no user could be parsed from the email address")
	}
	newUsr := &DBUser{
		Id:               newUsername,
		EmailAddress:     newEmail,
		FavoriteProjects: oldUsr.FavoriteProjects,
		PatchNumber:      oldUsr.PatchNumber,
		Settings:         oldUsr.Settings,
		SystemRoles:      oldUsr.Roles(),
		APIKey:           oldUsr.GetAPIKey(),
		PubKeys:          oldUsr.PublicKeys(),
	}

	_, err := UpsertOne(ctx, bson.M{IdKey: newUsername}, bson.M{"$set": bson.M{
		EmailAddressKey:     newUsr.Email(),
		FavoriteProjectsKey: newUsr.FavoriteProjects,
		PatchNumberKey:      newUsr.PatchNumber,
		SettingsKey:         newUsr.Settings,
		RolesKey:            newUsr.Roles(),
		APIKeyKey:           newUsr.GetAPIKey(),
		PubKeysKey:          newUsr.PublicKeys(),
	}})

	return newUsr, errors.Wrapf(err, "unable to insert new user '%s'", newUsername)
}
