package data

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type DBSubscriptionConnector struct{}

func (dc *DBSubscriptionConnector) SaveSubscriptions(owner string, subscriptions []restModel.APISubscription, isProjectOwner bool) error {
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
		if isProjectOwner {
			dbSubscription.OwnerType = event.OwnerTypeProject
			dbSubscription.Owner = owner
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

		if ok, msg := event.ValidateSelectors(dbSubscription.Selectors); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid selectors: %s", msg),
			}
		}
		if ok, msg := event.ValidateSelectors(dbSubscription.RegexSelectors); !ok {
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

		if dbSubscription.ResourceType == event.ResourceTypeVersion && isEndTrigger(dbSubscription.Trigger) {

			//find all children, iterate through them
			var versionId string
			for _, selector := range dbSubscription.Selectors {
				if selector.Type == "id" {
					versionId = selector.Data
				}
			}
			children, err := getVersionChildren(versionId)
			if err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    "Error retrieving child versions: " + err.Error(),
				}
			}

			for _, childPatchId := range children {
				childDbSubscription := dbSubscription
				childDbSubscription.LastUpdated = time.Now()
				var selectors []event.Selector
				for _, selector := range dbSubscription.Selectors {
					if selector.Type == "id" {
						selector.Data = childPatchId
					}
					selectors = append(selectors, selector)
				}
				childDbSubscription.Selectors = selectors
				dbSubscriptions = append(dbSubscriptions, childDbSubscription)
			}
		}

	}

	catcher := grip.NewSimpleCatcher()
	for _, subscription := range dbSubscriptions {
		catcher.Add(subscription.Upsert())
	}
	return catcher.Resolve()
}

func isEndTrigger(trigger string) bool {
	return trigger == event.TriggerFailure || trigger == event.TriggerSuccess || trigger == event.TriggerOutcome
}

func getVersionChildren(versionId string) ([]string, error) {
	patchDoc, err := patch.FindOne(patch.ByVersion(versionId))
	if err != nil {
		return nil, errors.Wrap(err, "error getting patch")
	}
	if patchDoc == nil {
		return nil, errors.Wrap(err, "patch not found")
	}
	return patchDoc.Triggers.ChildPatches, nil

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
	MockSubscriptions []restModel.APISubscription
}

func (mc *MockSubscriptionConnector) GetSubscriptions(owner string, ownerType event.OwnerType) ([]restModel.APISubscription, error) {
	return mc.MockSubscriptions, nil
}

func (mc *MockSubscriptionConnector) SaveSubscriptions(owner string, subscriptions []restModel.APISubscription, isProjectOwner bool) error {
	for _, sub := range subscriptions {
		mc.MockSubscriptions = append(mc.MockSubscriptions, sub)
	}
	return nil
}

func (mc *MockSubscriptionConnector) DeleteSubscriptions(owner string, ids []string) error {
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	n := 0
	for _, sub := range mc.MockSubscriptions {
		if idMap[utility.FromStringPtr(sub.ID)] {
			mc.MockSubscriptions[n] = sub
			n++
		}
	}
	mc.MockSubscriptions = mc.MockSubscriptions[:n]

	return nil
}

func (mc *MockSubscriptionConnector) CopyProjectSubscriptions(oldProject, newProject string) error {
	return nil
}
