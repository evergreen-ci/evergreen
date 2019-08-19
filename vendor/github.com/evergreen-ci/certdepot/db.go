package certdepot

import (
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// PutTTL sets the TTL to the given expiration time for the name. If the name is
// not found in the collection, this will error. The expiration must be within
// the validity bounds of the certificate for the given name.
func (m *mongoDepot) PutTTL(name string, expiration time.Time) error {
	expiration = expiration.UTC()

	minExpiration, maxExpiration, err := ValidityBounds(m, name)
	if err != nil {
		return errors.Wrap(err, "could not get certificate validity bounds")
	}
	if expiration.Before(minExpiration) || expiration.After(maxExpiration) {
		return errors.Errorf("cannot set expiration to %s because it must be between %s and %s", expiration, minExpiration, maxExpiration)
	}

	formattedName := strings.Replace(name, " ", "_", -1)
	updateRes, err := m.client.Database(m.databaseName).Collection(m.collectionName).UpdateOne(m.ctx,
		bson.M{userIDKey: formattedName},
		bson.M{"$set": bson.M{userTTLKey: expiration}})
	if err != nil {
		return errors.Wrap(err, "problem updating TTL in the database")
	}
	if updateRes.ModifiedCount == 0 {
		return errors.Errorf("update did not change TTL for user %s", name)
	}
	return nil
}

func (m *mongoDepot) GetTTL(name string) (time.Time, error) {
	formattedName := strings.Replace(name, " ", "_", -1)
	var user User
	if err := m.client.Database(m.databaseName).Collection(m.collectionName).FindOne(m.ctx,
		bson.M{userIDKey: formattedName},
	).Decode(&user); err != nil {
		return time.Time{}, errors.Wrap(err, "could not get TTL from database")
	}
	return user.TTL, nil
}

// FindExpiresBefore finds all Users that expire before the given cutoff time.
func (m *mongoDepot) FindExpiresBefore(cutoff time.Time) ([]User, error) {
	users := []User{}
	res, err := m.client.Database(m.databaseName).Collection(m.collectionName).
		Find(m.ctx, expiresBeforeQuery(cutoff))
	if err != nil {
		return nil, errors.Wrap(err, "problem finding expired users")
	}
	if err := res.All(m.ctx, &users); err != nil {
		return nil, errors.Wrap(err, "problem decoding results")
	}

	return users, nil
}

// DeleteExpiresBefore removes all Users that expire before the given cutoff
// time.
func (m *mongoDepot) DeleteExpiresBefore(cutoff time.Time) error {
	_, err := m.client.Database(m.databaseName).Collection(m.collectionName).
		DeleteMany(m.ctx, expiresBeforeQuery(cutoff))
	if err != nil {
		return errors.Wrap(err, "problem removing expired users")
	}
	return nil
}

func expiresBeforeQuery(cutoff time.Time) bson.M {
	return bson.M{userTTLKey: bson.M{"$lte": cutoff}}
}
