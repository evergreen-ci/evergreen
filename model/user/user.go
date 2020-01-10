package user

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mgobson "gopkg.in/mgo.v2/bson"
)

type DBUser struct {
	Id           string       `bson:"_id"`
	FirstName    string       `bson:"first_name"`
	LastName     string       `bson:"last_name"`
	DispName     string       `bson:"display_name"`
	EmailAddress string       `bson:"email"`
	PatchNumber  int          `bson:"patch_number"`
	PubKeys      []PubKey     `bson:"public_keys" json:"public_keys"`
	CreatedAt    time.Time    `bson:"created_at"`
	Settings     UserSettings `bson:"settings"`
	APIKey       string       `bson:"apikey"`
	SystemRoles  []string     `bson:"roles"`
	LoginCache   LoginCache   `bson:"login_cache,omitempty"`
}

func (u *DBUser) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(u) }
func (u *DBUser) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, u) }

type LoginCache struct {
	Token        string    `bson:"token"`
	TTL          time.Time `bson:"ttl"`
	AccessToken  string    `bson:"access_token,omitempty"`
	RefreshToken string    `bson:"refresh_token,omitempty"`
}

type GithubUser struct {
	UID         int    `bson:"uid,omitempty" json:"uid,omitempty"`
	LastKnownAs string `bson:"last_known_as,omitempty" json:"last_known_as,omitempty"`
}

type PubKey struct {
	Name      string    `bson:"name" json:"name"`
	Key       string    `bson:"key" json:"key"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

type UserSettings struct {
	Timezone         string                  `json:"timezone" bson:"timezone"`
	GithubUser       GithubUser              `json:"github_user" bson:"github_user,omitempty"`
	SlackUsername    string                  `bson:"slack_username,omitempty" json:"slack_username,omitempty"`
	Notifications    NotificationPreferences `bson:"notifications,omitempty" json:"notifications,omitempty"`
	UseSpruceOptions UseSpruceOptions        `json:"use_spruce_options" bson:"use_spruce_options"`
}

type UseSpruceOptions struct {
	PatchPage bool `json:"patch_page" bson:"patch_page"`
}

type NotificationPreferences struct {
	BuildBreak            UserSubscriptionPreference `bson:"build_break" json:"build_break"`
	BuildBreakID          string                     `bson:"build_break_id,omitempty" json:"-"`
	PatchFinish           UserSubscriptionPreference `bson:"patch_finish" json:"patch_finish"`
	PatchFinishID         string                     `bson:"patch_finish_id,omitempty" json:"-"`
	SpawnHostExpiration   UserSubscriptionPreference `bson:"spawn_host_expiration" json:"spawn_host_expiration"`
	SpawnHostExpirationID string                     `bson:"spawn_host_expiration_id,omitempty" json:"-"`
	SpawnHostOutcome      UserSubscriptionPreference `bson:"spawn_host_outcome" json:"spawn_host_outcome"`
	SpawnHostOutcomeID    string                     `bson:"spawn_host_outcome_id,omitempty" json:"-"`
	CommitQueue           UserSubscriptionPreference `bson:"commit_queue" json:"commit_queue"`
	CommitQueueID         string                     `bson:"commit_queue_id,omitempty" json:"-"`
}

type UserSubscriptionPreference string

const (
	PreferenceEmail UserSubscriptionPreference = event.EmailSubscriberType
	PreferenceSlack UserSubscriptionPreference = event.SlackSubscriberType
)

func (u *DBUser) Username() string     { return u.Id }
func (u *DBUser) PublicKeys() []PubKey { return u.PubKeys }
func (u *DBUser) Email() string        { return u.EmailAddress }
func (u *DBUser) GetAPIKey() string    { return u.APIKey }
func (u *DBUser) IsNil() bool          { return u == nil }

func (u *DBUser) Roles() []string {
	if u.SystemRoles == nil {
		return []string{}
	}
	return u.SystemRoles
}

func (u *DBUser) DisplayName() string {
	if u.DispName != "" {
		return u.DispName
	}
	return u.Id
}

func (u *DBUser) GetAccessToken() string {
	grip.Alert("GetAccessToken not yet implemented for DBUser")
	return ""
}

func (u *DBUser) GetRefreshToken() string {
	grip.Alert("GetRefreshToken not yet implemented for DBUser")
	return ""
}

func (u *DBUser) GetPublicKey(keyname string) (string, error) {
	for _, publicKey := range u.PubKeys {
		if publicKey.Name == keyname {
			return publicKey.Key, nil
		}
	}
	return "", errors.Errorf("Unable to find public key '%v' for user '%v'", keyname, u.Username())
}

func (u *DBUser) AddPublicKey(keyName, keyValue string) error {
	key := PubKey{
		Name:      keyName,
		Key:       keyValue,
		CreatedAt: time.Now(),
	}
	userWithoutKey := bson.M{
		IdKey: u.Id,
		bsonutil.GetDottedKeyName(PubKeysKey, PubKeyNameKey): bson.M{"$ne": keyName},
	}
	update := bson.M{
		"$push": bson.M{PubKeysKey: key},
	}

	if err := UpdateOne(userWithoutKey, update); err != nil {
		return err
	}

	u.PubKeys = append(u.PubKeys, key)
	return nil
}

func (u *DBUser) DeletePublicKey(keyName string) error {
	newUser := DBUser{}

	selector := bson.M{
		IdKey: u.Id,
		bsonutil.GetDottedKeyName(PubKeysKey, PubKeyNameKey): bson.M{"$eq": keyName},
	}
	c := adb.Change{
		Update: bson.M{
			"$pull": bson.M{
				PubKeysKey: bson.M{
					PubKeyNameKey: keyName,
				},
			},
		},
		ReturnNew: true,
	}
	change, err := db.FindAndModify(Collection, selector, nil, c, &newUser)

	if err != nil {
		return errors.Wrap(err, "couldn't delete public key from user")
	}
	if change.Updated != 1 {
		return errors.Errorf("public key deletion query succeeded but unexpected ChangeInfo: %+v", change)
	}
	u.PubKeys = newUser.PubKeys
	return nil
}

func (u *DBUser) Insert() error {
	u.CreatedAt = time.Now()
	return db.Insert(Collection, u)
}

// IncPatchNumber increases the count for the user's patch submissions by one,
// and then returns the new count.
func (u *DBUser) IncPatchNumber() (int, error) {
	dbUser := &DBUser{}
	_, err := db.FindAndModify(
		Collection,
		bson.M{
			IdKey: u.Id,
		},
		nil,
		adb.Change{
			Update: bson.M{
				"$inc": bson.M{
					PatchNumberKey: 1,
				},
			},
			Upsert:    true,
			ReturnNew: true,
		},
		dbUser,
	)
	if err != nil {
		return 0, err
	}
	return dbUser.PatchNumber, nil
}

func (u *DBUser) AddRole(role string) error {
	if util.StringSliceContains(u.SystemRoles, role) {
		return errors.Errorf("cannot add duplicate role '%s'", role)
	}
	update := bson.M{
		"$push": bson.M{RolesKey: role},
	}
	if err := UpdateOne(bson.M{IdKey: u.Id}, update); err != nil {
		return err
	}
	u.SystemRoles = append(u.SystemRoles, role)

	return event.LogUserEvent(u.Id, event.UserEventTypeRolesUpdate, u.SystemRoles[:len(u.SystemRoles)-1], u.SystemRoles)
}

func (u *DBUser) RemoveRole(role string) error {
	before := u.SystemRoles
	update := bson.M{
		"$pull": bson.M{RolesKey: role},
	}
	if err := UpdateOne(bson.M{IdKey: u.Id}, update); err != nil {
		return err
	}
	for i := len(u.SystemRoles) - 1; i >= 0; i-- {
		if u.SystemRoles[i] == role {
			u.SystemRoles = append(u.SystemRoles[:i], u.SystemRoles[i+1:]...)
		}
	}

	return event.LogUserEvent(u.Id, event.UserEventTypeRolesUpdate, before, u.SystemRoles)
}

func (u *DBUser) HasPermission(opts gimlet.PermissionOpts) bool {
	roleManager := evergreen.GetEnvironment().RoleManager()
	roles, err := roleManager.GetRoles(u.Roles())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting roles",
		}))
		return false
	}
	roles, err = roleManager.FilterForResource(roles, opts.Resource, opts.ResourceType)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error filtering resources",
		}))
		return false
	}
	for _, role := range roles {
		level, hasPermission := role.Permissions[opts.Permission]
		if hasPermission && level >= opts.RequiredLevel {
			return true
		}
	}
	return false
}

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
				APIKey:   util.RandomString(),
			}
			if err = u.Insert(); err != nil {
				return nil, errors.Wrap(err, "failed to create github patch user")
			}
		}
	}

	return u, nil
}

func IsValidSubscriptionPreference(in string) bool {
	switch in {
	case event.EmailSubscriberType, event.SlackSubscriberType, "", event.SubscriberTypeNone:
		return true
	default:
		return false
	}
}

func FormatObjectID(id string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(id)
}

// PutLoginCache generates a token if one does not exist, and sets the TTL to now.
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
	var update bson.M
	if token == "" {
		token = util.RandomString()
		update = bson.M{"$set": bson.M{
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTokenKey): token,
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTTLKey):   time.Now(),
		}}
	} else {
		update = bson.M{"$set": bson.M{
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTTLKey): time.Now(),
		}}
	}

	if err := UpdateOne(bson.M{IdKey: u.Id}, update); err != nil {
		return "", errors.Wrap(err, "problem updating user cache")
	}
	return token, nil
}

// PutLoginCacheAndTokens is the same as PutLoginCache but also adds the given
// access and refresh tokens to the cache.
func PutLoginCacheAndTokens(gu gimlet.User, accessToken, refreshToken string) (string, error) {
	u, err := FindOneById(gu.Username())
	if err != nil {
		return "", errors.Wrap(err, "problem finding user by id")
	}
	if u == nil {
		return "", errors.Errorf("no user '%s' found", gu.Username())
	}

	// Always update the TTL. If the user doesn't  have a token, generate and
	// set it.
	var update bson.M
	token := u.LoginCache.Token
	if token == "" {
		token = util.RandomString()
		update = bson.M{"$set": bson.M{
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTokenKey):        token,
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTTLKey):          time.Now(),
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheAccessTokenKey):  accessToken,
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheRefreshTokenKey): refreshToken,
		}}
	} else {
		update = bson.M{"$set": bson.M{
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheTTLKey):          time.Now(),
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheAccessTokenKey):  accessToken,
			bsonutil.GetDottedKeyName(LoginCacheKey, LoginCacheRefreshTokenKey): refreshToken,
		}}
	}
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

// GetLoginCacheAndTokens is the same as GetLoginCache but also returns their
// access and refresh tokens.
func GetLoginCacheAndTokens(token string, expireAfter time.Duration) (user gimlet.User, valid bool, accessToken string, refreshToken string, err error) {
	u, err := FindOneByToken(token)
	if err != nil {
		return nil, false, "", "", errors.Wrap(err, "probloem getting user from cache")
	}
	if u == nil {
		return nil, false, "", "", nil
	}
	return u, time.Since(u.LoginCache.TTL) < expireAfter, u.LoginCache.AccessToken, u.LoginCache.RefreshToken, nil
}

// ClearLoginCache clears a user or all users' tokens from the cache, forcibly logging them out
func ClearLoginCache(user gimlet.User, all bool) error {
	update := bson.M{"$unset": bson.M{LoginCacheKey: 1}}

	if all {
		query := bson.M{}
		if err := UpdateAll(query, update); err != nil {
			return errors.Wrap(err, "problem updating user cache")
		}
	} else {
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
	}
	return nil
}
