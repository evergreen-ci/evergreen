package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type DBSubscriptionConnector struct{}

func (dc *DBSubscriptionConnector) SaveSubscriptions(subscriptions []event.Subscription) error {
	catcher := grip.NewSimpleCatcher()
	for _, subscription := range subscriptions {
		catcher.Add(subscription.Upsert())
	}
	return catcher.Resolve()
}

func (dc *DBSubscriptionConnector) GetSubscriptions(owner string, ownerType event.OwnerType) ([]restModel.APISubscription, error) {
	if len(owner) == 0 {
		return nil, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "no subscription owner provided",
		}
	}

	subs, err := event.FindSubscriptionsByOwner(owner, ownerType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}

	apiSubs := make([]restModel.APISubscription, len(subs))

	for i := range subs {
		err = apiSubs[i].BuildFromService(subs[i])
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal subscriptions")
		}
	}

	return apiSubs, nil
}

type MockSubscriptionConnector struct {
	MockSubscriptions []event.Subscription
}

func (mc *MockSubscriptionConnector) GetSubscriptions(user string, ownerType event.OwnerType) ([]restModel.APISubscription, error) {
	return nil, errors.New("MockSubscriptionConnector unimplemented")
}

func (mc *MockSubscriptionConnector) SaveSubscriptions(subscriptions []event.Subscription) error {
	return errors.New("MockSubscriptionConnector unimplemented")
}
