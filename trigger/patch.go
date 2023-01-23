package trigger

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
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
		event.TriggerFamilyOutcome: t.patchFamilyOutcome,
		event.TriggerFamilyFailure: t.patchFamilyFailure,
		event.TriggerFamilySuccess: t.patchFamilySuccess,
		event.TriggerOutcome:       t.patchOutcome,
		event.TriggerFailure:       t.patchFailure,
		event.TriggerSuccess:       t.patchSuccess,
		event.TriggerPatchStarted:  t.patchStarted,
	}
	return t
}

func (t *patchTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(evergreen.GetEnvironment()); err != nil {
		return errors.Wrap(err, "fetching UI config")
	}

	oid := mgobson.ObjectIdHex(e.ResourceId)

	t.patch, err = patch.FindOne(patch.ById(oid))
	if err != nil {
		return errors.Wrapf(err, "finding patch '%s'", e.ResourceId)
	}
	if t.patch == nil {
		return errors.Errorf("patch '%s' not found", e.ResourceId)
	}
	var ok bool
	t.data, ok = e.Data.(*event.PatchEventData)
	if !ok {
		return errors.Errorf("patch '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *patchTriggers) Attributes() event.Attributes {
	owner := []string{t.patch.Author}
	if t.event.EventType == event.PatchChildrenCompletion {
		eventData := t.event.Data.(*event.PatchEventData)
		owner = []string{eventData.Author}
	}
	return event.Attributes{
		ID:      []string{t.patch.Id.Hex()},
		Object:  []string{event.ObjectPatch},
		Project: []string{t.patch.Project},
		Owner:   owner,
		Status:  []string{t.patch.Status},
	}
}

func (t *patchTriggers) patchOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if (t.data.Status != evergreen.PatchSucceeded && t.data.Status != evergreen.PatchFailed) || t.event.EventType == event.PatchChildrenCompletion {
		return nil, nil
	}

	if sub.Subscriber.Type == event.RunChildPatchSubscriberType {
		target, ok := sub.Subscriber.Target.(*event.ChildPatchSubscriber)
		if !ok {
			return nil, errors.Errorf("target '%s' had unexpected type %T", sub.Subscriber.Target, sub.Subscriber.Target)
		}
		ps := target.ParentStatus

		if ps != evergreen.PatchSucceeded && ps != evergreen.PatchFailed && ps != evergreen.PatchAllOutcomes {
			return nil, nil
		}

		successOutcome := (ps == evergreen.PatchSucceeded) && (t.data.Status == evergreen.PatchSucceeded)
		failureOutcome := (ps == evergreen.PatchFailed) && (t.data.Status == evergreen.PatchFailed)
		anyOutcome := ps == evergreen.PatchAllOutcomes

		if successOutcome || failureOutcome || anyOutcome {
			aborted, err := model.IsAborted(t.patch.Id.Hex())
			if err != nil {
				return nil, errors.Wrapf(err, "getting aborted status for patch '%s'", t.patch.Id.Hex())
			}
			if aborted {
				return nil, nil
			}
			err = finalizeChildPatch(sub)

			if err != nil {
				return nil, errors.Wrap(err, "finalizing child patch")
			}
			return nil, nil
		}
	}
	return t.generate(sub)
}

func (t *patchTriggers) patchFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchFailed || t.event.EventType == event.PatchChildrenCompletion {
		return nil, nil
	}

	return t.generate(sub)
}

func finalizeChildPatch(sub *event.Subscription) error {
	target, ok := sub.Subscriber.Target.(*event.ChildPatchSubscriber)
	if !ok {
		return errors.Errorf("target '%s' had unexpected type %T", sub.Subscriber.Target, sub.Subscriber.Target)
	}
	childPatch, err := patch.FindOneId(target.ChildPatchId)
	if err != nil {
		return errors.Wrap(err, "finding child patch")
	}
	if childPatch == nil {
		return errors.Errorf("child patch '%s' not found", target.ChildPatchId)
	}
	// Return if patch is already finalized
	if childPatch.Version != "" {
		return nil
	}

	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()

	if _, err := model.FinalizePatch(ctx, childPatch, target.Requester, ""); err != nil {
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
	if t.data.Status != evergreen.PatchSucceeded || t.event.EventType == event.PatchChildrenCompletion {
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
	if err := api.BuildFromService(*t.patch, &restModel.APIPatchArgs{
		IncludeProjectIdentifier: true,
	}); err != nil {
		return nil, errors.Wrap(err, "building patch args from service model")
	}
	projectName := t.patch.Project
	if api.ProjectIdentifier != nil {
		projectName = utility.FromStringPtr(api.ProjectIdentifier)
	}
	collectiveStatus, err := patch.CollectiveStatus(t.patch.Id.Hex())
	if err != nil {
		return nil, errors.Wrap(err, "getting collective status for patch")
	}
	grip.NoticeWhen(collectiveStatus != t.data.Status, message.Fields{
		"message":                 "patch's current collective status does not match the patch event data's status",
		"patch_collective_status": collectiveStatus,
		"patch_status":            t.patch.Status,
		"patch_event_status":      t.data.Status,
		"patch":                   t.patch.Id.Hex(),
		"subscription":            sub.ID,
	})

	data := commonTemplateData{
		ID:                t.patch.Id.Hex(),
		EventID:           t.event.ID,
		SubscriptionID:    sub.ID,
		DisplayName:       t.patch.Id.Hex(),
		Description:       t.patch.Description,
		Object:            event.ObjectPatch,
		Project:           projectName,
		PastTenseStatus:   collectiveStatus,
		apiModel:          &api,
		githubState:       message.GithubStatePending,
		githubDescription: "tasks are running",
	}

	if t.patch.IsChild() {
		githubContext, err := t.getGithubContext()
		if err != nil {
			return nil, errors.Wrapf(err, "getting GitHub context for patch '%s'", t.patch.Id)
		}
		data.githubContext = githubContext
		data.URL = versionLink(
			versionLinkInput{
				uiBase:    t.uiConfig.UIv2Url,
				versionID: t.patch.Triggers.ParentPatch,
				hasPatch:  false,
				isChild:   true,
			},
		)
	} else {
		data.githubContext = "evergreen"
		data.URL = versionLink(
			versionLinkInput{
				uiBase:    t.uiConfig.Url,
				versionID: t.patch.Version,
				hasPatch:  true,
				isChild:   false,
			},
		)
	}

	slackColor := evergreenFailColor
	finishTime := t.patch.FinishTime
	if utility.IsZeroTime(finishTime) {
		finishTime = time.Now()
	}

	if collectiveStatus == evergreen.PatchSucceeded {
		slackColor = evergreenSuccessColor
		data.githubState = message.GithubStateSuccess
		data.githubDescription = fmt.Sprintf("patch finished in %s", finishTime.Sub(t.patch.StartTime).String())
	} else if collectiveStatus == evergreen.PatchFailed {
		data.githubState = message.GithubStateFailure
		data.githubDescription = fmt.Sprintf("patch finished in %s", finishTime.Sub(t.patch.StartTime).String())
	}

	if t.patch.IsGithubPRPatch() {
		data.slack = append(data.slack, message.SlackAttachment{
			Title:     "GitHub Pull Request",
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
		return nil, errors.Wrap(err, "collecting patch data")
	}

	payload, err := makeCommonPayload(sub, t.Attributes(), data)
	if err != nil {
		return nil, errors.Wrap(err, "building notification")
	}
	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *patchTriggers) getGithubContext() (string, error) {
	projectIdentifier, err := model.GetIdentifierForProject(t.patch.Project)
	if err != nil { // default to ID
		projectIdentifier = t.patch.Project
	}

	parentPatch, err := patch.FindOneId(t.patch.Triggers.ParentPatch)
	if err != nil {
		return "", errors.Wrapf(err, "getting parent patch '%s'", t.patch.Triggers.ParentPatch)
	}
	if parentPatch == nil {
		return "", errors.Errorf("parent patch '%s' not found", t.patch.Triggers.ParentPatch)
	}
	patchIndex, err := t.patch.GetPatchIndex(parentPatch)
	if err != nil {
		return "", errors.Wrap(err, "getting child patch index")
	}
	var githubContext string
	if patchIndex == 0 || patchIndex == -1 {
		githubContext = fmt.Sprintf("evergreen/%s", projectIdentifier)
	} else {
		githubContext = fmt.Sprintf("evergreen/%s/%d", projectIdentifier, patchIndex)
	}
	return githubContext, nil
}

func (t *patchTriggers) patchFamilyOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchSucceeded && t.data.Status != evergreen.PatchFailed {
		return nil, nil
	}
	if t.event.EventType != event.PatchChildrenCompletion {
		return nil, nil
	}

	// Don't notify the user of the patch outcome if they aborted the patch
	aborted, err := model.IsAborted(t.patch.Id.Hex())
	if err != nil {
		return nil, errors.Wrapf(err, "getting aborted status for patch '%s'", t.patch.Id.Hex())
	}
	if aborted {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchFamilySuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchSucceeded || t.event.EventType != event.PatchChildrenCompletion {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchFamilyFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchFailed || t.event.EventType != event.PatchChildrenCompletion {
		return nil, nil
	}
	return t.generate(sub)
}
