package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SaveUserSettings updates the settings stored for the given user id.
func SaveUserSettings(userId string, settings user.UserSettings) error {
	update := bson.M{"$set": bson.M{user.SettingsKey: settings}}
	return errors.Wrapf(user.UpdateOne(bson.M{user.IdKey: userId}, update), "problem saving user settings for %s", userId)
}

// SetUserAPIKey updates the API key stored with a user.
func SetUserAPIKey(userId, newKey string) error {
	update := bson.M{"$set": bson.M{user.APIKeyKey: newKey}}
	return errors.Wrapf(user.UpdateOne(bson.M{user.IdKey: userId}, update), "problem setting api key for user %s", userId)
}

func FindUserByID(id string) (*user.DBUser, error) {
	t, err := user.FindOne(user.ById(id))
	if err != nil {
		return nil, errors.Wrapf(err, "db issue finding user '%s'", id)
	}
	if t == nil {
		return nil, errors.Errorf("user %s not found", id)
	}
	return t, nil
}

// GetOrCreateUser fetches a user with the given userId and returns it. If no document exists for
// that userId, inserts it along with the provided display name and email.
func GetOrCreateUser(userId, displayName, email string) (*user.DBUser, error) {
	u := &user.DBUser{}
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	res := env.DB().Collection(user.Collection).FindOneAndUpdate(ctx,
		bson.M{user.IdKey: userId},
		bson.M{
			"$set": bson.M{
				user.DispNameKey:     displayName,
				user.EmailAddressKey: email,
			},
			"$setOnInsert": bson.M{
				user.APIKeyKey: util.RandomString(),
			},
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
