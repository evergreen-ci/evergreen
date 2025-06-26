package evergreen

import (
	"context"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// AuthUser configures a user for our Naive authentication setup.
type AuthUser struct {
	Username    string `bson:"username" json:"username" yaml:"username"`
	DisplayName string `bson:"display_name" json:"display_name" yaml:"display_name"`
	Password    string `bson:"password" json:"password" yaml:"password"`
	Email       string `bson:"email" json:"email" yaml:"email"`
}

var (
	AuthOktaKey                    = bsonutil.MustHaveTag(AuthConfig{}, "Okta")
	AuthGithubKey                  = bsonutil.MustHaveTag(AuthConfig{}, "Github")
	AuthNaiveKey                   = bsonutil.MustHaveTag(AuthConfig{}, "Naive")
	AuthMultiKey                   = bsonutil.MustHaveTag(AuthConfig{}, "Multi")
	AuthKanopyKey                  = bsonutil.MustHaveTag(AuthConfig{}, "Kanopy")
	authPreferredTypeKey           = bsonutil.MustHaveTag(AuthConfig{}, "PreferredType")
	authBackgroundReauthMinutesKey = bsonutil.MustHaveTag(AuthConfig{}, "BackgroundReauthMinutes")
	AuthAllowServiceUsersKey       = bsonutil.MustHaveTag(AuthConfig{}, "AllowServiceUsers")
)

// NaiveAuthConfig contains a list of AuthUsers from the settings file.
type NaiveAuthConfig struct {
	Users []AuthUser `bson:"users" json:"users" yaml:"users"`
}

type OktaConfig struct {
	ClientID           string   `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret       string   `bson:"client_secret" json:"client_secret" yaml:"client_secret" secret:"true"`
	Issuer             string   `bson:"issuer" json:"issuer" yaml:"issuer"`
	Scopes             []string `bson:"scopes" json:"scopes" yaml:"scopes"`
	UserGroup          string   `bson:"user_group" json:"user_group" yaml:"user_group"`
	ExpireAfterMinutes int      `bson:"expire_after_minutes" json:"expire_after_minutes" yaml:"expire_after_minutes"`
}

// GithubAuthConfig contains settings for interacting with Github Authentication
// including the ClientID, ClientSecret and CallbackUri which are given when
// registering the application Furthermore,
type GithubAuthConfig struct {
	AppId        int64    `bson:"app_id" json:"app_id" yaml:"app_id"`
	ClientId     string   `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret string   `bson:"client_secret" json:"client_secret" yaml:"client_secret"`
	DefaultOwner string   `bson:"default_owner" json:"default_owner" yaml:"default_owner"`
	DefaultRepo  string   `bson:"default_repo" json:"default_repo" yaml:"default_repo"`
	Organization string   `bson:"organization" json:"organization" yaml:"organization"`
	Users        []string `bson:"users" json:"users" yaml:"users"`
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

// KanopyAuthConfig configures the auth method that validates and consumes the JWT
// that Kanopy provides with information about the user. Kanopy deals with authentication
// so all we need to do is extract the information they provide about the user.
type KanopyAuthConfig struct {
	// HeaderName is the name of the header that contains the JWT with information about the user.
	HeaderName string `bson:"header_name" json:"header_name" yaml:"header_name"`
	// Issuer is the expected issuer of the JWT. JWT Validation fails if the JWT's issuer field
	// does not match the Issuer provided.
	Issuer string `bson:"issuer" json:"issuer" yaml:"issuer"`
	// KeysetURL is the URL for the remote keyset, or JWKS, used to validate the signing of the JWT.
	KeysetURL string `bson:"keyset_url" json:"keyset_url" yaml:"keyset_url"`
}

// AuthConfig contains the settings for the various auth managers.
type AuthConfig struct {
	Okta                    *OktaConfig       `bson:"okta,omitempty" json:"okta" yaml:"okta"`
	Naive                   *NaiveAuthConfig  `bson:"naive,omitempty" json:"naive" yaml:"naive"`
	Github                  *GithubAuthConfig `bson:"github,omitempty" json:"github" yaml:"github"`
	Multi                   *MultiAuthConfig  `bson:"multi" json:"multi" yaml:"multi"`
	Kanopy                  *KanopyAuthConfig `bson:"kanopy" json:"kanopy" yaml:"kanopy"`
	AllowServiceUsers       bool              `bson:"allow_service_users" json:"allow_service_users" yaml:"allow_service_users"`
	PreferredType           string            `bson:"preferred_type,omitempty" json:"preferred_type" yaml:"preferred_type"`
	BackgroundReauthMinutes int               `bson:"background_reauth_minutes" json:"background_reauth_minutes" yaml:"background_reauth_minutes"`
}

func (c *AuthConfig) SectionId() string { return "auth" }

func (c *AuthConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *AuthConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			AuthOktaKey:                    c.Okta,
			AuthNaiveKey:                   c.Naive,
			AuthGithubKey:                  c.Github,
			AuthMultiKey:                   c.Multi,
			AuthKanopyKey:                  c.Kanopy,
			authPreferredTypeKey:           c.PreferredType,
			authBackgroundReauthMinutesKey: c.BackgroundReauthMinutes,
			AuthAllowServiceUsersKey:       c.AllowServiceUsers,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *AuthConfig) checkDuplicateUsers() error {
	catcher := grip.NewBasicCatcher()
	var usernames []string
	if c.Naive != nil {
		for _, u := range c.Naive.Users {
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
		AuthOktaKey,
		AuthNaiveKey,
		AuthGithubKey,
		AuthMultiKey,
		AuthKanopyKey,
	}, c.PreferredType), "invalid auth type '%s'", c.PreferredType)

	if c.Naive == nil && c.Github == nil && c.Okta == nil && c.Multi == nil && c.Kanopy == nil {
		catcher.Add(errors.New("must specify one form of authentication"))
	}

	catcher.Add(c.checkDuplicateUsers())

	if c.Multi != nil {
		seen := map[string]bool{}
		kinds := append([]string{}, c.Multi.ReadWrite...)
		kinds = append(kinds, c.Multi.ReadOnly...)
		for _, kind := range kinds {
			// Check that settings exist for the user manager.
			switch kind {
			case AuthOktaKey:
				catcher.NewWhen(c.Okta == nil, "Okta settings cannot be empty if using in multi auth")
			case AuthGithubKey:
				catcher.NewWhen(c.Github == nil, "GitHub settings cannot be empty if using in multi auth")
			case AuthNaiveKey:
				catcher.NewWhen(c.Naive == nil, "Naive settings cannot be empty if using in multi auth")
			default:
				catcher.Errorf("unrecognized auth mechanism '%s'", kind)
			}
			// Check for duplicate user managers.
			catcher.ErrorfWhen(seen[kind], "duplicate auth mechanism '%s' in multi auth", kind)
			seen[kind] = true
		}
	}

	if c.Kanopy != nil {
		catcher.NewWhen(c.Kanopy.HeaderName == "", "header name cannot be empty if using Kanopy auth")
		catcher.NewWhen(c.Kanopy.Issuer == "", "issuer cannot be empty if using Kanopy auth")
		catcher.NewWhen(c.Kanopy.KeysetURL == "", "keyset URL cannot be empty if using Kanopy auth")
	}

	return catcher.Resolve()
}
