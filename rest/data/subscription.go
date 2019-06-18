package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
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
		return nil, gimlet.ErrorResponse{
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

func (dc *DBSubscriptionConnector) DeleteSubscription(id string) error {
	return event.RemoveSubscription(id)
}

func (dc *DBSubscriptionConnector) CopyProjectSubscriptions(oldProject, newProject string) error {
	catcher := grip.NewBasicCatcher()
	subs, err := event.FindSubscriptionsByOwner(oldProject, event.OwnerTypeProject)
	if err != nil {
		return errors.Wrapf(err, "error finding subscription for project '%s'", oldProject)
	}

	for _, sub := range subs {
		sub.Owner = newProject
		sub.ID = ""
		catcher.Add(sub.Upsert())
	}
	return catcher.Resolve()
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

func (dc *MockSubscriptionConnector) DeleteSubscription(id string) error {
	return errors.New("MockSubscriptionConnector unimplemented")
}

func (dc *MockSubscriptionConnector) CopyProjectSubscriptions(oldProject, newProject string) error {
	return nil
}
