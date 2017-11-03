package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// SaveUserSettings updates the settings stored for the given user id.
func SaveUserSettings(userId string, settings user.UserSettings) error {
	update := bson.M{"$set": bson.M{user.SettingsKey: settings}}
	return user.UpdateOne(bson.M{user.IdKey: userId}, update)
}

// SetUserAPIKey updates the API key stored with a user.
func SetUserAPIKey(userId, newKey string) error {
	update := bson.M{"$set": bson.M{user.APIKeyKey: newKey}}
	return user.UpdateOne(bson.M{user.IdKey: userId}, update)
}

// GetOrCreateUser fetches a user with the given userId and returns it. If no document exists for
// that userId, inserts it along with the provided display name and email.
func GetOrCreateUser(userId, displayName, email string) (*user.DBUser, error) {
	u := &user.DBUser{}
	_, err := db.FindAndModify(user.Collection, bson.M{user.IdKey: userId}, nil,
		mgo.Change{
			Update: bson.M{
				"$set": bson.M{
					user.DispNameKey:     displayName,
					user.EmailAddressKey: email,
				},
				"$setOnInsert": bson.M{
					user.APIKeyKey: util.RandomString(),
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
