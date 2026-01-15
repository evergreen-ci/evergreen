package user

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type DBUser struct {
	Id                     string                 `bson:"_id"`
	FirstName              string                 `bson:"first_name"`
	LastName               string                 `bson:"last_name"`
	DispName               string                 `bson:"display_name"`
	EmailAddress           string                 `bson:"email"`
	PatchNumber            int                    `bson:"patch_number"`
	PubKeys                []PubKey               `bson:"public_keys" json:"public_keys"`
	CreatedAt              time.Time              `bson:"created_at"`
	Settings               UserSettings           `bson:"settings"`
	APIKey                 string                 `bson:"apikey"`
	SystemRoles            []string               `bson:"roles"`
	LoginCache             LoginCache             `bson:"login_cache,omitempty"`
	FavoriteProjects       []string               `bson:"favorite_projects"`
	OnlyAPI                bool                   `bson:"only_api,omitempty"`
	ParsleyFilters         []parsley.Filter       `bson:"parsley_filters"`
	ParsleySettings        parsley.Settings       `bson:"parsley_settings"`
	NumScheduledPatchTasks int                    `bson:"num_scheduled_patch_tasks"`
	LastScheduledTasksAt   time.Time              `bson:"last_scheduled_tasks_at"`
	BetaFeatures           evergreen.BetaFeatures `bson:"beta_features"`
}

func (u *DBUser) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(u) }
func (u *DBUser) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, u) }

type LoginCache struct {
	Token        string    `bson:"token,omitempty"`
	TTL          time.Time `bson:"ttl,omitempty"`
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
	Region           string                  `json:"region" bson:"region"`
	GithubUser       GithubUser              `json:"github_user" bson:"github_user,omitempty"`
	SlackUsername    string                  `bson:"slack_username,omitempty" json:"slack_username,omitempty"`
	SlackMemberId    string                  `bson:"slack_member_id,omitempty" json:"slack_member_id,omitempty"`
	Notifications    NotificationPreferences `bson:"notifications,omitempty" json:"notifications"`
	UseSpruceOptions UseSpruceOptions        `json:"use_spruce_options" bson:"use_spruce_options"`
	DateFormat       string                  `json:"date_format" bson:"date_format"`
	TimeFormat       string                  `json:"time_format" bson:"time_format"`
}

type UseSpruceOptions struct {
	SpruceV1 bool `json:"spruce_v1" bson:"spruce_v1"` // represents users opted into the new Evergreen UI
}

type NotificationPreferences struct {
	BuildBreak            UserSubscriptionPreference `bson:"build_break" json:"build_break"`
	BuildBreakID          string                     `bson:"build_break_id,omitempty" json:"-"`
	PatchFinish           UserSubscriptionPreference `bson:"patch_finish" json:"patch_finish"`
	PatchFinishID         string                     `bson:"patch_finish_id,omitempty" json:"-"`
	PatchFirstFailure     UserSubscriptionPreference `bson:"patch_first_failure,omitempty" json:"patch_first_failure"`
	PatchFirstFailureID   string                     `bson:"patch_first_failure_id,omitempty" json:"-"`
	SpawnHostExpiration   UserSubscriptionPreference `bson:"spawn_host_expiration" json:"spawn_host_expiration"`
	SpawnHostExpirationID string                     `bson:"spawn_host_expiration_id,omitempty" json:"-"`
	SpawnHostOutcome      UserSubscriptionPreference `bson:"spawn_host_outcome" json:"spawn_host_outcome"`
	SpawnHostOutcomeID    string                     `bson:"spawn_host_outcome_id,omitempty" json:"-"`
}

type UserSubscriptionPreference string

const (
	PreferenceEmail UserSubscriptionPreference = event.EmailSubscriberType
	PreferenceSlack UserSubscriptionPreference = event.SlackSubscriberType
	PreferenceNone  UserSubscriptionPreference = event.SubscriberTypeNone
)

func (u *DBUser) Username() string        { return u.Id }
func (u *DBUser) PublicKeys() []PubKey    { return u.PubKeys }
func (u *DBUser) Email() string           { return u.EmailAddress }
func (u *DBUser) GetAPIKey() string       { return u.APIKey }
func (u *DBUser) GetAccessToken() string  { return u.LoginCache.AccessToken }
func (u *DBUser) GetRefreshToken() string { return u.LoginCache.RefreshToken }
func (u *DBUser) IsAPIOnly() bool         { return u.OnlyAPI }
func (u *DBUser) IsNil() bool             { return u == nil }

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

func (u *DBUser) GetRegion() string {
	return u.Settings.Region
}

func (u *DBUser) GetPublicKey(keyname string) (string, error) {
	for _, publicKey := range u.PubKeys {
		if publicKey.Name == keyname {
			return publicKey.Key, nil
		}
	}
	return "", errors.Errorf("Unable to find public key '%v' for user '%v'", keyname, u.Username())
}

// UpdateAPIKey updates the API key stored for the user.
func (u *DBUser) UpdateAPIKey(ctx context.Context, newKey string) error {
	update := bson.M{"$set": bson.M{APIKeyKey: newKey}}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return errors.Wrapf(err, "setting API key for user '%s'", u.Id)
	}
	u.APIKey = newKey
	return nil
}

// UpdateSettings updates the user's settings.
func (u *DBUser) UpdateSettings(ctx context.Context, settings UserSettings) error {
	update := bson.M{"$set": bson.M{SettingsKey: settings}}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return errors.Wrapf(err, "saving user settings for user '%s'", u.Id)
	}
	u.Settings = settings
	return nil
}

// UpdateParsleySettings updates a user's settings for Parsley.
func (u *DBUser) UpdateParsleySettings(ctx context.Context, settings parsley.Settings) error {
	update := bson.M{"$set": bson.M{ParsleySettingsKey: settings}}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return errors.Wrapf(err, "saving Parsley settings for user '%s'", u.Id)
	}
	u.ParsleySettings = settings
	return nil
}

// UpdateBetaFeatures updates a user's beta feature settings.
func (u *DBUser) UpdateBetaFeatures(ctx context.Context, betaFeatures evergreen.BetaFeatures) error {
	update := bson.M{"$set": bson.M{BetaFeaturesKey: betaFeatures}}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return errors.Wrapf(err, "saving beta feature settings for user '%s'", u.Id)
	}
	u.BetaFeatures = betaFeatures
	return nil
}

// CheckAndUpdateSchedulingLimit checks if the number of tasks to be activated by the user is allowed given
// the global per-user hourly task scheduling limit, and updates relevant timestamp and counter info used
// to track the user's hourly scheduling usage. The activated parameter being false signifies
// the user is deactivating tasks, which frees up space in their scheduling limit.
func (u *DBUser) CheckAndUpdateSchedulingLimit(ctx context.Context, maxScheduledTasks, numTasksModified int, activated bool) error {
	var update bson.M
	if activated && numTasksModified > maxScheduledTasks {
		return errors.Errorf("cannot schedule %d tasks, maximum hourly per-user limit is %d", numTasksModified, maxScheduledTasks)
	}
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	// If the last time the user scheduled patch tasks was within the hour, increment the number
	// of activated tasks to the user's counter, erroring if the global limit is breached. If numTasksModified
	// is negative, the counter is decremented.
	if u.LastScheduledTasksAt.After(oneHourAgo) {
		update = bson.M{
			"$set": bson.M{NumScheduledPatchTasksKey: getNewNumScheduledTasksCounter(u.NumScheduledPatchTasks, numTasksModified, activated)},
		}
		if activated && (numTasksModified+u.NumScheduledPatchTasks) >= maxScheduledTasks {
			minutesRemaining := 60 - int(now.Sub(u.LastScheduledTasksAt).Minutes())
			return errors.Errorf("user '%s' has scheduled %d out of %d allowed tasks in the past hour, limit refreshes in %d minutes", u.Id, u.NumScheduledPatchTasks, maxScheduledTasks, minutesRemaining)
		}
	} else {
		// Otherwise, if the user has not scheduled any patch tasks within the past hour, reset the last scheduled tasks
		// timestamp to now, and reset the number of schedule tasks to the number of activated tasks passed in here.
		update = bson.M{
			"$set": bson.M{
				NumScheduledPatchTasksKey: getNewNumScheduledTasksCounter(0, numTasksModified, activated),
				LastScheduledTasksAtKey:   time.Now(),
			},
		}
	}
	return UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update)
}

// getNewNumScheduledTasksCounter takes in the current number of tasks a user has scheduled within the
// last hour and updates it with the number of tasks that have been activated or deactivated, indicated
// by the activated parameter.
func getNewNumScheduledTasksCounter(currentCounter, numTasksModified int, activated bool) int {
	if activated {
		return currentCounter + numTasksModified
	}
	subtraction := currentCounter - numTasksModified
	if subtraction < 0 {
		return 0
	}
	return subtraction
}

func (u *DBUser) AddPublicKey(ctx context.Context, keyName, keyValue string) error {
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
	// If the DB state is nil, we need to use a $set operation rather than a $push.
	if u.PubKeys == nil {
		update = bson.M{
			"$set": bson.M{PubKeysKey: []PubKey{key}},
		}
	}
	if err := UpdateOneContext(ctx, userWithoutKey, update); err != nil {
		return err
	}
	u.PubKeys = append(u.PubKeys, key)
	return nil
}

func (u *DBUser) DeletePublicKey(ctx context.Context, keyName string) error {
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
	change, err := db.FindAndModify(ctx, Collection, selector, nil, c, &newUser)

	if err != nil {
		return errors.Wrap(err, "couldn't delete public key from user")
	}
	if change.Updated != 1 {
		return errors.Errorf("public key deletion query succeeded but unexpected ChangeInfo: %+v", change)
	}
	u.PubKeys = newUser.PubKeys
	return nil
}

func (u *DBUser) UpdatePublicKey(ctx context.Context, targetKeyName, newKeyName, newKeyValue string) error {
	newUser := DBUser{}
	targetKeySelector := bson.M{
		IdKey: u.Id,
		bsonutil.GetDottedKeyName(PubKeysKey, PubKeyNameKey): bson.M{"$eq": targetKeyName},
	}
	updatedKey := PubKey{
		Name:      newKeyName,
		Key:       newKeyValue,
		CreatedAt: time.Now(),
	}
	c := adb.Change{
		Update: bson.M{
			"$set": bson.M{
				bsonutil.GetDottedKeyName(PubKeysKey, "$"): updatedKey,
			},
		},
		ReturnNew: true,
	}
	change, err := db.FindAndModify(ctx, Collection, targetKeySelector, nil, c, &newUser)
	if err != nil {
		return errors.Wrap(err, "updating public key from user")
	}
	if change.Updated != 1 {
		return errors.Errorf("public key update query expected to update exactly one user, but instead updated %d", change.Updated)
	}
	u.PubKeys = newUser.PubKeys
	return nil
}

func (u *DBUser) Insert(ctx context.Context) error {
	u.CreatedAt = time.Now()
	return db.Insert(ctx, Collection, u)
}

// IncPatchNumber increases the count for the user's patch submissions by one,
// and then returns the new count.
func (u *DBUser) IncPatchNumber(ctx context.Context) (int, error) {
	dbUser := &DBUser{}
	_, err := db.FindAndModify(ctx,
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

// AddFavoritedProject adds a project ID to the user favorites in user DB model
func (u *DBUser) AddFavoritedProject(ctx context.Context, identifier string) error {
	if utility.StringSliceContains(u.FavoriteProjects, identifier) {
		return errors.Errorf("cannot add duplicate project '%s'", identifier)
	}
	update := bson.M{
		"$push": bson.M{FavoriteProjectsKey: identifier},
	}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return err
	}

	u.FavoriteProjects = append(u.FavoriteProjects, identifier)

	return nil
}

// RemoveFavoriteProject removes a project ID from the user favorites in user DB model
func (u *DBUser) RemoveFavoriteProject(ctx context.Context, identifier string) error {
	if !utility.StringSliceContains(u.FavoriteProjects, identifier) {
		return errors.Errorf("project '%s' does not exist in user's favorites", identifier)
	}

	update := bson.M{
		"$pull": bson.M{FavoriteProjectsKey: identifier},
	}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return err
	}

	for i := len(u.FavoriteProjects) - 1; i >= 0; i-- {
		if u.FavoriteProjects[i] == identifier {
			u.FavoriteProjects = append(u.FavoriteProjects[:i], u.FavoriteProjects[i+1:]...)
		}
	}

	return nil
}

func (u *DBUser) AddRole(ctx context.Context, role string) error {
	if utility.StringSliceContains(u.SystemRoles, role) {
		return nil
	}
	update := bson.M{
		"$addToSet": bson.M{RolesKey: role},
	}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return err
	}
	u.SystemRoles = append(u.SystemRoles, role)

	return event.LogUserEvent(ctx, u.Id, event.UserEventTypeRolesUpdate, u.SystemRoles[:len(u.SystemRoles)-1], u.SystemRoles)
}

func (u *DBUser) RemoveRole(ctx context.Context, role string) error {
	before := u.SystemRoles
	update := bson.M{
		"$pull": bson.M{RolesKey: role},
	}
	if err := UpdateOneContext(ctx, bson.M{IdKey: u.Id}, update); err != nil {
		return err
	}
	for i := len(u.SystemRoles) - 1; i >= 0; i-- {
		if u.SystemRoles[i] == role {
			u.SystemRoles = append(u.SystemRoles[:i], u.SystemRoles[i+1:]...)
		}
	}

	return event.LogUserEvent(ctx, u.Id, event.UserEventTypeRolesUpdate, before, u.SystemRoles)
}

// GetViewableProjects returns the lists of projects/repos the user can view settings for.
func (u *DBUser) GetViewableProjectSettings(ctx context.Context) ([]string, error) {
	if evergreen.PermissionsDisabledForTests() {
		return nil, nil
	}
	roleManager := evergreen.GetEnvironment().RoleManager()

	viewableProjects, err := rolemanager.FindAllowedResources(ctx, roleManager, u.Roles(), evergreen.ProjectResourceType, evergreen.PermissionProjectSettings, evergreen.ProjectSettingsView.Value)
	if err != nil {
		return nil, err
	}
	return viewableProjects, nil
}

// GetViewableProjects returns the lists of projects the user can view.
func (u *DBUser) GetViewableProjects(ctx context.Context) ([]string, error) {
	if evergreen.PermissionsDisabledForTests() {
		return nil, nil
	}
	roleManager := evergreen.GetEnvironment().RoleManager()

	viewableProjects, err := rolemanager.FindAllowedResources(ctx, roleManager, u.Roles(), evergreen.ProjectResourceType, evergreen.PermissionTasks, evergreen.TasksView.Value)
	if err != nil {
		return nil, err
	}
	return viewableProjects, nil
}

func (u *DBUser) HasPermission(opts gimlet.PermissionOpts) bool {
	if evergreen.PermissionsDisabledForTests() {
		return true
	}
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

// HasProjectCreatePermission returns true if the user is an admin for any existing project.
func (u *DBUser) HasProjectCreatePermission() (bool, error) {
	roleManager := evergreen.GetEnvironment().RoleManager()
	roles, err := roleManager.GetRoles(u.Roles())
	if err != nil {
		return false, errors.Wrap(err, "getting roles")
	}
	for _, role := range roles {
		level, hasPermission := role.Permissions[evergreen.PermissionProjectSettings]
		if hasPermission && level >= evergreen.ProjectSettingsEdit.Value {
			return true, nil
		}
	}
	return false, nil
}

// HasDistroCreatePermission returns true if the user has permission to create
// distros. This can also operate as a check for whether the user is a distro
// admin, since only distro admins can create new distros.
func (u *DBUser) HasDistroCreatePermission() bool {
	return u.HasPermission(gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionDistroCreate,
		RequiredLevel: evergreen.DistroCreate.Value,
	})
}

func (u *DBUser) DeleteAllRoles(ctx context.Context) error {
	info, err := db.FindAndModify(ctx,
		Collection,
		bson.M{IdKey: u.Id},
		nil,
		adb.Change{
			Update: bson.M{
				"$set": bson.M{RolesKey: []string{}},
			},
		}, u)
	if err != nil {
		return errors.Wrap(err, "clearing user roles")
	}
	if info.Updated != 1 {
		return errors.Errorf("could not find user '%s' to update", u.Id)
	}
	return nil
}

func (u *DBUser) DeleteRoles(ctx context.Context, roles []string) error {
	if len(roles) == 0 {
		return nil
	}
	info, err := db.FindAndModify(ctx,
		Collection,
		bson.M{IdKey: u.Id},
		nil,
		adb.Change{
			Update: bson.M{
				"$pullAll": bson.M{RolesKey: roles},
			},
		}, u)
	if err != nil {
		return errors.Wrap(err, "deleting user roles")
	}
	if info.Updated != 1 {
		return errors.Errorf("could not find user '%s' to update", u.Id)
	}
	return nil
}

// GeneralSubscriptionIDs returns a slice of the ids of the user's general subscriptions.
func (u *DBUser) GeneralSubscriptionIDs() []string {
	var ids []string
	if id := u.Settings.Notifications.BuildBreakID; id != "" {
		ids = append(ids, id)
	}
	if id := u.Settings.Notifications.PatchFinishID; id != "" {
		ids = append(ids, id)
	}
	if id := u.Settings.Notifications.PatchFirstFailureID; id != "" {
		ids = append(ids, id)
	}
	if id := u.Settings.Notifications.SpawnHostExpirationID; id != "" {
		ids = append(ids, id)
	}
	if id := u.Settings.Notifications.SpawnHostOutcomeID; id != "" {
		ids = append(ids, id)
	}

	return ids
}

func IsValidSubscriptionPreference(in string) bool {
	switch in {
	case event.EmailSubscriberType, event.SlackSubscriberType, "", event.SubscriberTypeNone:
		return true
	default:
		return false
	}
}
