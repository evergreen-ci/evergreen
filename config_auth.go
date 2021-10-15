package evergreen

import (
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AuthUser configures a user for our Naive authentication setup.
type AuthUser struct {
	Username    string `bson:"username" json:"username" yaml:"username"`
	DisplayName string `bson:"display_name" json:"display_name" yaml:"display_name"`
	Password    string `bson:"password" json:"password" yaml:"password"`
	Email       string `bson:"email" json:"email" yaml:"email"`
}

var (
	AuthLDAPKey                    = bsonutil.MustHaveTag(AuthConfig{}, "LDAP")
	AuthOktaKey                    = bsonutil.MustHaveTag(AuthConfig{}, "Okta")
	AuthGithubKey                  = bsonutil.MustHaveTag(AuthConfig{}, "Github")
	AuthNaiveKey                   = bsonutil.MustHaveTag(AuthConfig{}, "Naive")
	AuthOnlyAPIKey                 = bsonutil.MustHaveTag(AuthConfig{}, "OnlyAPI")
	AuthMultiKey                   = bsonutil.MustHaveTag(AuthConfig{}, "Multi")
	authPreferredTypeKey           = bsonutil.MustHaveTag(AuthConfig{}, "PreferredType")
	authBackgroundReauthMinutesKey = bsonutil.MustHaveTag(AuthConfig{}, "BackgroundReauthMinutes")
	AuthAllowServiceUsersKey       = bsonutil.MustHaveTag(AuthConfig{}, "AllowServiceUsers")
)

// OnlyAPIUser configures a special service user with only access to the API via
// a key.
type OnlyAPIUser struct {
	Username string   `bson:"username" json:"username" yaml:"username"`
	Key      string   `bson:"key" json:"key" yaml:"key"`
	Roles    []string `bson:"roles" json:"roles" yaml:"roles"`
}

// NaiveAuthConfig contains a list of AuthUsers from the settings file.
type NaiveAuthConfig struct {
	Users []AuthUser `bson:"users" json:"users" yaml:"users"`
}

// OnlyAPIAuthConfig contains the users that can only authenticate via the API from
// the settings.
type OnlyAPIAuthConfig struct {
	Users []OnlyAPIUser `bson:"users" json:"users" yaml:"users"`
}

// LDAPConfig contains settings for interacting with an LDAP server.
type LDAPConfig struct {
	URL                string `bson:"url" json:"url" yaml:"url"`
	Port               string `bson:"port" json:"port" yaml:"port"`
	UserPath           string `bson:"path" json:"path" yaml:"path"`
	ServicePath        string `bson:"service_path" json:"service_path" yaml:"service_path"`
	Group              string `bson:"group" json:"group" yaml:"group"`
	ServiceGroup       string `bson:"service_group" json:"service_group" yaml:"service_group"`
	ExpireAfterMinutes string `bson:"expire_after_minutes" json:"expire_after_minutes" yaml:"expire_after_minutes"`
	GroupOU            string `bson:"group_ou" json:"group_ou" yaml:"group_ou"`
}

type OktaConfig struct {
	ClientID           string   `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret       string   `bson:"client_secret" json:"client_secret" yaml:"client_secret"`
	Issuer             string   `bson:"issuer" json:"issuer" yaml:"issuer"`
	Scopes             []string `bson:"scopes" json:"scopes" yaml:"scopes"`
	UserGroup          string   `bson:"user_group" json:"user_group" yaml:"user_group"`
	ExpireAfterMinutes int      `bson:"expire_after_minutes" json:"expire_after_minutes" yaml:"expire_after_minutes"`
}

// GithubAuthConfig contains settings for interacting with Github Authentication
// including the ClientID, ClientSecret and CallbackUri which are given when
// registering the application Furthermore,
type GithubAuthConfig struct {
	ClientId     string   `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret string   `bson:"client_secret" json:"client_secret" yaml:"client_secret"`
	Users        []string `bson:"users" json:"users" yaml:"users"`
	Organization string   `bson:"organization" json:"organization" yaml:"organization"`
}

// MultiAuthConfig contains settings for using multiple authentication
// mechanisms.
type MultiAuthConfig struct {
	ReadWrite []string `bson:"read_write" json:"read_write" yaml:"read_write"`
	ReadOnly  []string `bson:"read_only" json:"read_only" yaml:"read_only"`
}

// IsZero checks if the configuration is populated or not.
func (c *MultiAuthConfig) IsZero() bool {
	return len(c.ReadWrite) == 0 && len(c.ReadOnly) == 0
}

// AuthConfig contains the settings for the various auth managers.
type AuthConfig struct {
	LDAP                    *LDAPConfig        `bson:"ldap,omitempty" json:"ldap" yaml:"ldap"`
	Okta                    *OktaConfig        `bson:"okta,omitempty" json:"okta" yaml:"okta"`
	Naive                   *NaiveAuthConfig   `bson:"naive,omitempty" json:"naive" yaml:"naive"`
	OnlyAPI                 *OnlyAPIAuthConfig `bson:"only_api,omitempty" json:"only_api" yaml:"only_api"` // deprecated
	Github                  *GithubAuthConfig  `bson:"github,omitempty" json:"github" yaml:"github"`
	Multi                   *MultiAuthConfig   `bson:"multi" json:"multi" yaml:"multi"`
	AllowServiceUsers       bool               `bson:"allow_service_users" json:"allow_service_users" yaml:"allow_service_users"`
	PreferredType           string             `bson:"preferred_type,omitempty" json:"preferred_type" yaml:"preferred_type"`
	BackgroundReauthMinutes int                `bson:"background_reauth_minutes" json:"background_reauth_minutes" yaml:"background_reauth_minutes"`
}

func (c *AuthConfig) SectionId() string { return "auth" }

func (c *AuthConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = AuthConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	// Clear the struct because Decode will not set fields that are omitempty to
	// the zero value if they're zero in the database.
	*c = AuthConfig{}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *AuthConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			AuthLDAPKey:                    c.LDAP,
			AuthOktaKey:                    c.Okta,
			AuthNaiveKey:                   c.Naive,
			AuthOnlyAPIKey:                 c.OnlyAPI,
			AuthGithubKey:                  c.Github,
			AuthMultiKey:                   c.Multi,
			authPreferredTypeKey:           c.PreferredType,
			authBackgroundReauthMinutesKey: c.BackgroundReauthMinutes,
			AuthAllowServiceUsersKey:       c.AllowServiceUsers,
		}}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *AuthConfig) checkDuplicateUsers() error {
	catcher := grip.NewBasicCatcher()
	var usernames []string
	if c.Naive != nil {
		for _, u := range c.Naive.Users {
			usernames = append(usernames, u.Username)
		}
	}
	if c.OnlyAPI != nil {
		for _, u := range c.OnlyAPI.Users {
			usernames = append(usernames, u.Username)
		}
	}
	used := map[string]bool{}
	for _, name := range usernames {
		catcher.AddWhen(used[name], errors.Errorf("duplicate user '%s' in list", name))
		used[name] = true
	}
	return catcher.Resolve()
}

func (c *AuthConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	catcher.ErrorfWhen(!utility.StringSliceContains([]string{
		"",
		AuthLDAPKey,
		AuthOktaKey,
		AuthNaiveKey,
		AuthGithubKey,
		AuthMultiKey}, c.PreferredType), "invalid auth type '%s'", c.PreferredType)

	if c.LDAP == nil && c.Naive == nil && c.OnlyAPI == nil && c.Github == nil && c.Okta == nil && c.Multi == nil {
		catcher.Add(errors.New("You must specify one form of authentication"))
	}

	catcher.Add(c.checkDuplicateUsers())

	if c.OnlyAPI != nil {
		// Generate API key if none are explicitly set.
		for i := range c.OnlyAPI.Users {
			if c.OnlyAPI.Users[i].Key == "" {
				c.OnlyAPI.Users[i].Key = utility.RandomString()
			}
		}
	}

	if c.Github != nil {
		if c.Github.Users == nil && c.Github.Organization == "" {
			catcher.Add(errors.New("Must specify either a set of users or an organization for Github Authentication"))
		}
	}

	if c.Multi != nil {
		seen := map[string]bool{}
		kinds := append([]string{}, c.Multi.ReadWrite...)
		kinds = append(kinds, c.Multi.ReadOnly...)
		for _, kind := range kinds {
			// Check that settings exist for the user manager.
			switch kind {
			case AuthLDAPKey:
				catcher.NewWhen(c.LDAP == nil, "LDAP settings cannot be empty if using in multi auth")
			case AuthOktaKey:
				catcher.NewWhen(c.Okta == nil, "Okta settings cannot be empty if using in multi auth")
			case AuthGithubKey:
				catcher.NewWhen(c.Github == nil, "GitHub settings cannot be empty if using in multi auth")
			case AuthNaiveKey:
				catcher.NewWhen(c.Naive == nil, "Naive settings cannot be empty if using in multi auth")
			case AuthOnlyAPIKey:
				continue
			default:
				catcher.Errorf("unrecognized auth mechanism '%s'", kind)
			}
			// Check for duplicate user managers.
			catcher.ErrorfWhen(seen[kind], "duplicate auth mechanism '%s' in multi auth", kind)
			seen[kind] = true
		}
	}

	return catcher.Resolve()
}
