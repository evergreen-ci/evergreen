package evergreen

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/util"
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

// NaiveAuthConfig contains a list of AuthUsers from the settings file.
type NaiveAuthConfig struct {
	Users []*AuthUser `bson:"users" json:"users" yaml:"users"`
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
	ClientID           string `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret       string `bson:"client_secret" json:"client_secret" yaml:"client_secret"`
	Issuer             string `bson:"issuer" json:"issuer" yaml:"issuer"`
	UserGroup          string `bson:"user_group" json:"user_group" yaml:"user_group"`
	ExpireAfterMinutes int    `bson:"expire_after_minutes" json:"expire_after_minutes" yaml:"expire_after_minutes"`
}

// GithubAuthConfig holds settings for interacting with Github Authentication including the
// ClientID, ClientSecret and CallbackUri which are given when registering the application
// Furthermore,
type GithubAuthConfig struct {
	ClientId     string   `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret string   `bson:"client_secret" json:"client_secret" yaml:"client_secret"`
	Users        []string `bson:"users" json:"users" yaml:"users"`
	Organization string   `bson:"organization" json:"organization" yaml:"organization"`
}

// AuthConfig has a pointer to either a CrowConfig or a NaiveAuthConfig.
type AuthConfig struct {
	LDAP          *LDAPConfig       `bson:"ldap,omitempty" json:"ldap" yaml:"ldap"`
	Okta          *OktaConfig       `bson:"okta,omitempty" json:"okta" yaml:"okta"`
	Naive         *NaiveAuthConfig  `bson:"naive,omitempty" json:"naive" yaml:"naive"`
	Github        *GithubAuthConfig `bson:"github,omitempty" json:"github" yaml:"github"`
	PreferredType string            `bson:"preferred_type,omitempty" json:"preferred_type" yaml:"preferred_type"`
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
			AuthLDAPKey:          c.LDAP,
			AuthOktaKey:          c.Okta,
			AuthNaiveKey:         c.Naive,
			AuthGithubKey:        c.Github,
			authPreferredTypeKey: c.PreferredType,
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *AuthConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	catcher.ErrorfWhen(!util.StringSliceContains([]string{
		"",
		AuthLDAPKey,
		AuthOktaKey,
		AuthNaiveKey,
		AuthGithubKey}, c.PreferredType), "invalid auth type '%s'", c.PreferredType)
	if c.LDAP == nil && c.Naive == nil && c.Github == nil && c.Okta == nil {
		catcher.Add(errors.New("You must specify one form of authentication"))
	}
	if c.Naive != nil {
		used := map[string]bool{}
		for _, x := range c.Naive.Users {
			if used[x.Username] {
				catcher.Add(fmt.Errorf("Duplicate user %s in list", x.Username))
			}
			used[x.Username] = true
		}
	}
	if c.Github != nil {
		if c.Github.Users == nil && c.Github.Organization == "" {
			catcher.Add(errors.New("Must specify either a set of users or an organization for Github Authentication"))
		}
	}
	return catcher.Resolve()
}
