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
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func SaveSubscriptions(owner string, subscriptions []restModel.APISubscription, isProjectOwner bool) error {
	dbSubscriptions := []event.Subscription{}
	for _, subscription := range subscriptions {
		subscriptionInterface, err := subscription.ToService()
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "converting subscription to service model").Error(),
			}
		}
		dbSubscription, ok := subscriptionInterface.(event.Subscription)
		if !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("programmatic error: expected subscription model type but actually got type %T", subscriptionInterface),
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
				Message:    "cannot change subscriptions for anyone other than yourself",
			}
		}

		if ok, msg := event.IsSubscriptionAllowed(dbSubscription); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    msg,
			}
		}

		err = dbSubscription.Validate()
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "invalid subscription").Error(),
			}
		}

		dbSubscriptions = append(dbSubscriptions, dbSubscription)

		if dbSubscription.ResourceType == event.ResourceTypeVersion && isEndTrigger(dbSubscription.Trigger) {
			var versionId string
			for _, selector := range dbSubscription.Selectors {
				if selector.Type == event.SelectorID {
					versionId = selector.Data
				}
			}
			children, err := getVersionChildren(versionId)
			if err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrapf(err, "retrieving child versions for version '%s'", versionId).Error(),
				}
			}

			for _, childPatchId := range children {
				childDbSubscription := dbSubscription
				childDbSubscription.LastUpdated = time.Now()
				childDbSubscription.Filter.ID = childPatchId
				var selectors []event.Selector
				for _, selector := range dbSubscription.Selectors {
					if selector.Type == event.SelectorID {
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
		return nil, errors.Wrapf(err, "finding patch for version '%s'", versionId)
	}
	if patchDoc == nil {
		return nil, errors.Wrapf(err, "patch for version '%s' not found", versionId)
	}
	return patchDoc.Triggers.ChildPatches, nil

}

// GetSubscriptions returns the subscriptions that belong to a user
func GetSubscriptions(owner string, ownerType event.OwnerType) ([]restModel.APISubscription, error) {
	if len(owner) == 0 {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no subscription owner provided",
		}
	}

	subs, err := event.FindSubscriptionsByOwner(owner, ownerType)
	if err != nil {
		return nil, errors.Wrapf(err, "finding subscriptions for user '%s'", owner)
	}

	apiSubs := make([]restModel.APISubscription, len(subs))

	for i := range subs {
		err = apiSubs[i].BuildFromService(subs[i])
		if err != nil {
			return nil, errors.Wrapf(err, "converting subscription '%s' to API model", subs[i].ID)
		}
	}

	return apiSubs, nil
}

func DeleteSubscriptions(owner string, ids []string) error {
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
				Message:    "subscription not found",
			}
		}
		if subscription.Owner != owner {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "cannot delete subscriptions for someone other than yourself",
			}
		}
	}

	catcher := grip.NewBasicCatcher()
	for _, id := range ids {
		catcher.Wrapf(event.RemoveSubscription(id), "removing subscription '%s'", id)
	}
	return catcher.Resolve()
}
