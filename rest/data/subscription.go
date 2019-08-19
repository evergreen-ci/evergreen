package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type DBSubscriptionConnector struct{}

func (dc *DBSubscriptionConnector) SaveSubscriptions(owner string, subscriptions []restModel.APISubscription) error {
	dbSubscriptions := []event.Subscription{}
	for _, subscription := range subscriptions {
		subscriptionInterface, err := subscription.ToService()
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Error parsing request body: " + err.Error(),
			}
		}

		dbSubscription, ok := subscriptionInterface.(event.Subscription)
		if !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Error parsing subscription interface",
			}
		}

		if !trigger.ValidateTrigger(dbSubscription.ResourceType, dbSubscription.Trigger) {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("subscription type/trigger is invalid: %s/%s", dbSubscription.ResourceType, dbSubscription.Trigger),
			}
		}

		if dbSubscription.OwnerType == event.OwnerTypePerson && dbSubscription.Owner == "" {
			dbSubscription.Owner = owner // default the current user
		}

		if dbSubscription.OwnerType == event.OwnerTypePerson && dbSubscription.Owner != owner {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "Cannot change subscriptions for anyone other than yourself",
			}
		}

		if ok, msg := event.IsSubscriptionAllowed(dbSubscription); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    msg,
			}
		}

		if ok, msg := event.ValidateSelectors(dbSubscription.Subscriber, dbSubscription.Selectors); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid selectors: %s", msg),
			}
		}
		if ok, msg := event.ValidateSelectors(dbSubscription.Subscriber, dbSubscription.RegexSelectors); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid regex selectors: %s", msg),
			}
		}

		err = dbSubscription.Validate()
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Error validating subscription: " + err.Error(),
			}
		}

		dbSubscriptions = append(dbSubscriptions, dbSubscription)
	}

	catcher := grip.NewSimpleCatcher()
	for _, subscription := range dbSubscriptions {
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

func (dc *DBSubscriptionConnector) DeleteSubscriptions(owner string, ids []string) error {
	for _, id := range ids {
		subscription, err := event.FindSubscriptionByID(id)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
		if subscription == nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Subscription not found",
			}
		}
		if subscription.Owner != owner {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "Cannot delete subscriptions for someone other than yourself",
			}
		}
	}

	catcher := grip.NewBasicCatcher()
	for _, id := range ids {
		catcher.Add(event.RemoveSubscription(id))
	}
	return catcher.Resolve()
}

func (dc *DBSubscriptionConnector) CopyProjectSubscriptions(oldProject, newProject string) error {
	subs, err := event.FindSubscriptionsByOwner(oldProject, event.OwnerTypeProject)
	if err != nil {
		return errors.Wrapf(err, "error finding subscription for project '%s'", oldProject)
	}

	catcher := grip.NewBasicCatcher()
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

func (mc *MockSubscriptionConnector) GetSubscriptions(owner string, ownerType event.OwnerType) ([]restModel.APISubscription, error) {
	return nil, nil
}

func (mc *MockSubscriptionConnector) SaveSubscriptions(owner string, subscriptions []restModel.APISubscription) error {
	if len(subscriptions) == 0 {
		return nil
	}
	return errors.New("MockSubscriptionConnector unimplemented")
}

func (dc *MockSubscriptionConnector) DeleteSubscriptions(owner string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return errors.New("MockSubscriptionConnector unimplemented")
}

func (dc *MockSubscriptionConnector) CopyProjectSubscriptions(oldProject, newProject string) error {
	return nil
}
