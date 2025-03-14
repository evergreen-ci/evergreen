package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// There are special requirements for the VERSION resource type. Patch requesters should use family triggers
// when possible, while non-patch requesters should use regular triggers. The content of selectors changes based
// on the owner type.
func convertVersionSubscription(s *event.Subscription) error {
	var requester string

	if s.OwnerType == event.OwnerTypeProject { // Handle project subscriptions.
		for _, selector := range s.Selectors {
			if selector.Type == event.SelectorRequester {
				requester = selector.Data
				break
			}
		}
	} else if s.OwnerType == event.OwnerTypePerson { // Handle personal subscriptions.
		for _, selector := range s.Selectors {
			if selector.Type == event.SelectorID {
				versionId := selector.Data
				v, err := model.VersionFindOneId(versionId)
				if err != nil {
					return errors.Wrapf(err, "retrieving version '%s'", versionId)
				}
				if v == nil {
					return errors.Errorf("version '%s' not found", versionId)
				}
				requester = v.Requester
				break
			}
		}
	}

	if evergreen.IsPatchRequester(requester) {
		s.Trigger = trigger.ConvertToFamilyTrigger(s.Trigger)
	}
	return nil
}

func SaveSubscriptions(owner string, subscriptions []restModel.APISubscription, isProjectOwner bool) error {
	dbSubscriptions := []event.Subscription{}
	for _, subscription := range subscriptions {
		dbSubscription, err := subscription.ToService()
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrap(err, "converting subscription to service model").Error(),
			}
		}
		if isProjectOwner {
			dbSubscription.OwnerType = event.OwnerTypeProject
			dbSubscription.Owner = owner
		}

		if dbSubscription.ResourceType == event.ResourceTypeVersion {
			if err = convertVersionSubscription(&dbSubscription); err != nil {
				return gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "converting version subscription").Error(),
				}
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
				Message:    "cannot change subscriptions for anyone other than yourself",
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

	}

	catcher := grip.NewSimpleCatcher()
	for _, subscription := range dbSubscriptions {
		catcher.Add(subscription.Upsert())
	}
	return catcher.Resolve()
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

func DeleteSubscriptions(ctx context.Context, owner string, ids []string) error {
	for _, id := range ids {
		subscription, err := event.FindSubscriptionByID(ctx, id)
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
		catcher.Wrapf(event.RemoveSubscription(ctx, id), "removing subscription '%s'", id)
	}
	return catcher.Resolve()
}
