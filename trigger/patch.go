package trigger

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

type patchTriggers struct {
	event    *event.EventLogEntry
	data     *event.PatchEventData
	patch    *patch.Patch
	uiConfig evergreen.UIConfig

	base
}

func makePatchTriggers() eventHandler {
	t := &patchTriggers{}
	t.base.triggers = map[string]trigger{
		event.TriggerOutcome:      t.patchOutcome,
		event.TriggerFailure:      t.patchFailure,
		event.TriggerSuccess:      t.patchSuccess,
		event.TriggerPatchStarted: t.patchStarted,
	}
	return t
}

func (t *patchTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(evergreen.GetEnvironment()); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	oid := mgobson.ObjectIdHex(e.ResourceId)

	t.patch, err = patch.FindOne(patch.ById(oid))
	if err != nil {
		return errors.Wrapf(err, "failed to fetch patch '%s'", e.ResourceId)
	}
	if t.patch == nil {
		return errors.Errorf("can't find patch '%s'", e.ResourceId)
	}
	var ok bool
	t.data, ok = e.Data.(*event.PatchEventData)
	if !ok {
		return errors.Errorf("patch '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *patchTriggers) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: event.SelectorID,
			Data: t.patch.Id.Hex(),
		},
		{
			Type: event.SelectorObject,
			Data: event.ObjectPatch,
		},
		{
			Type: event.SelectorProject,
			Data: t.patch.Project,
		},
		{
			Type: event.SelectorOwner,
			Data: t.patch.Author,
		},
		{
			Type: event.SelectorStatus,
			Data: t.patch.Status,
		},
	}
}

func (t *patchTriggers) patchOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchSucceeded && t.data.Status != evergreen.PatchFailed {
		return nil, nil
	}

	if sub.Subscriber.Type == event.RunChildPatchSubscriberType {
		target, ok := sub.Subscriber.Target.(*event.ChildPatchSubscriber)
		if !ok {
			return nil, errors.Errorf("target '%s' didn't not have expected type", sub.Subscriber.Target)
		}
		ps := target.ParentStatus

		if ps != evergreen.PatchSucceeded && ps != evergreen.PatchFailed && ps != evergreen.PatchAllOutcomes {
			return nil, nil
		}

		successOutcome := (ps == evergreen.PatchSucceeded) && (t.data.Status == evergreen.PatchSucceeded)
		failureOutcome := (ps == evergreen.PatchFailed) && (t.data.Status == evergreen.PatchFailed)
		anyOutcome := (ps == evergreen.PatchAllOutcomes)

		if successOutcome || failureOutcome || anyOutcome {
			err := finalizeChildPatch(sub)

			if err != nil {
				return nil, errors.Wrap(err, "Failed to finalize child patch")
			}
			return nil, nil
		}
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func finalizeChildPatch(sub *event.Subscription) error {
	target, ok := sub.Subscriber.Target.(*event.ChildPatchSubscriber)
	if !ok {
		return errors.Errorf("target '%s' didn't not have expected type", sub.Subscriber.Target)
	}
	childPatch, err := patch.FindOneId(target.ChildPatchId)
	if err != nil {
		return errors.Wrap(err, "Failed to fetch child patch")
	}
	if childPatch == nil {
		return errors.Wrap(err, "child patch not found")
	}
	conf, err := evergreen.GetConfig()
	if err != nil {
		return errors.Wrap(err, "can't get evergreen configuration")
	}

	ghToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "can't get Github OAuth token from configuration")
	}

	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()

	if _, err := model.FinalizePatch(ctx, childPatch, target.Requester, ghToken); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "Failed to finalize patch document",
			"source":        target.Requester,
			"patch_id":      childPatch.Id,
			"variants":      childPatch.BuildVariants,
			"tasks":         childPatch.Tasks,
			"variant_tasks": childPatch.VariantsTasks,
			"alias":         childPatch.Alias,
		}))
		return err
	}
	return nil
}

func (t *patchTriggers) patchSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchSucceeded {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchStarted(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchStarted {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) makeData(sub *event.Subscription) (*commonTemplateData, error) {
	api := restModel.APIPatch{}
	if err := api.BuildFromService(*t.patch); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:                t.patch.Id.Hex(),
		EventID:           t.event.ID,
		SubscriptionID:    sub.ID,
		DisplayName:       t.patch.Id.Hex(),
		Description:       t.patch.Description,
		Object:            event.ObjectPatch,
		Project:           t.patch.Project,
		URL:               versionLink(t.uiConfig.Url, t.patch.Version, true),
		PastTenseStatus:   t.data.Status,
		apiModel:          &api,
		githubState:       message.GithubStatePending,
		githubContext:     "evergreen",
		githubDescription: "tasks are running",
	}
	slackColor := evergreenFailColor
	finishTime := t.patch.FinishTime
	if utility.IsZeroTime(finishTime) {
		finishTime = time.Now()
	}
	if t.data.Status == evergreen.PatchSucceeded {
		slackColor = evergreenSuccessColor
		data.githubState = message.GithubStateSuccess
		data.githubDescription = fmt.Sprintf("patch finished in %s", finishTime.Sub(t.patch.StartTime).String())
	} else if t.data.Status == evergreen.PatchFailed {
		data.githubState = message.GithubStateFailure
		data.githubDescription = fmt.Sprintf("patch finished in %s", finishTime.Sub(t.patch.StartTime).String())
	}
	if t.patch.IsGithubPRPatch() {
		data.slack = append(data.slack, message.SlackAttachment{
			Title:     "Github Pull Request",
			TitleLink: fmt.Sprintf("https://github.com/%s/%s/pull/%d#partial-pull-merging", t.patch.GithubPatchData.BaseOwner, t.patch.GithubPatchData.BaseRepo, t.patch.GithubPatchData.PRNumber),
			Color:     slackColor,
		})
	}
	var makespan time.Duration
	if utility.IsZeroTime(t.patch.FinishTime) {
		patchTasks, err := task.Find(task.ByVersion(t.patch.Id.Hex()))
		if err == nil {
			_, makespan = task.GetTimeSpent(patchTasks)
		}
	} else {
		makespan = t.patch.FinishTime.Sub(t.patch.StartTime)
	}

	data.slack = append(data.slack, message.SlackAttachment{
		Title:     "Evergreen Patch",
		TitleLink: data.URL,
		Text:      t.patch.Description,
		Color:     slackColor,
		Fields: []*message.SlackAttachmentField{
			{
				Title: "Time Taken",
				Value: makespan.String(),
			},
		},
	})

	return &data, nil
}

func (t *patchTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	data, err := t.makeData(sub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect patch data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}
