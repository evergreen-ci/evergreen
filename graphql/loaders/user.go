package loaders

import (
	"context"
	"errors"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vikstrous/dataloadgen"
	"go.mongodb.org/mongo-driver/bson"
)

type userReader struct{}

func (u *userReader) getUsers(ctx context.Context, userIDs []string) (map[string]*restModel.APIDBUser, error) {
	query := db.Query(bson.M{user.IdKey: bson.M{"$in": userIDs}})
	users, err := user.Find(ctx, query)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "error fetching users in dataloader",
		}))
		return nil, &batchError{err: err}
	}

	userMap := make(map[string]*restModel.APIDBUser, len(users))
	for i := range users {
		apiUser := &restModel.APIDBUser{}
		apiUser.BuildFromService(users[i])
		userMap[users[i].Id] = apiUser
	}

	return userMap, nil
}

// GetUser returns a single user by ID efficiently using the dataloader.
// Returns nil if the user is not found (e.g. service users).
func GetUser(ctx context.Context, userID string) (*restModel.APIDBUser, error) {
	l := For(ctx)
	result, err := l.UserLoader.Load(ctx, userID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return result, err
}
