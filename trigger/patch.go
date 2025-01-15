package trigger

import (
	"context"
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
	"github.com/evergreen-ci/evergreen/thirdparty"
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

const patchAllOutcomes = "*"

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

func (t *patchTriggers) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(ctx); err != nil {
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

func (t *patchTriggers) patchOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) || t.event.EventType == event.PatchChildrenCompletion {
		return nil, nil
	}

	if sub.Subscriber.Type == event.RunChildPatchSubscriberType {
		target, ok := sub.Subscriber.Target.(*event.ChildPatchSubscriber)
		if !ok {
			return nil, errors.Errorf("target '%s' had unexpected type %T", sub.Subscriber.Target, sub.Subscriber.Target)
		}
		ps := target.ParentStatus

		if !evergreen.IsFinishedVersionStatus(t.data.Status) && ps != patchAllOutcomes {
			return nil, nil
		}

		successOutcome := ps == evergreen.VersionSucceeded && t.data.Status == evergreen.VersionSucceeded
		failureOutcome := (ps == evergreen.VersionFailed) && (t.data.Status == evergreen.VersionFailed)
		anyOutcome := ps == patchAllOutcomes

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

func (t *patchTriggers) patchFailure(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed || t.event.EventType == event.PatchChildrenCompletion {
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

	if _, err := model.FinalizePatch(ctx, childPatch, target.Requester); err != nil {
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

func (t *patchTriggers) patchSuccess(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded || t.event.EventType == event.PatchChildrenCompletion {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchStarted(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionStarted {
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

	// For child patches, we only want to look at its own status because the collective status will be affected
	// by other child patches' and the parent's status
	collectiveStatus := t.data.Status
	if t.patch.IsParent() {
		var err error
		collectiveStatus, err = t.patch.CollectiveStatus()
		if err != nil {
			return nil, errors.Wrapf(err, "getting collective patch status for patch '%s'", t.patch.Id)
		}
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
		githubDescription: evergreen.PRTasksRunningDescription,
	}

	if t.patch.IsChild() {
		githubContext, err := t.getGithubContext(projectName)
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
		data.githubContext = thirdparty.GithubStatusDefaultContext
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

	if collectiveStatus == evergreen.VersionSucceeded {
		data.PastTenseStatus = "succeeded"
		slackColor = evergreenSuccessColor
		data.githubState = message.GithubStateSuccess
		data.githubDescription = fmt.Sprintf("patch finished in %s", finishTime.Sub(t.patch.StartTime).String())
	} else if collectiveStatus == evergreen.VersionFailed {
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

	tasks, err := task.Find(task.ByVersionWithChildTasks(t.patch.Id.Hex()))
	if err != nil {
		return nil, errors.Wrapf(err, "getting tasks for patch '%s'", t.patch.Id)
	}
	if tasks == nil {
		return nil, errors.Errorf("no tasks found for patch '%s'", t.patch.Id)
	}
	_, makespan := task.GetFormattedTimeSpent(tasks)

	data.slack = append(data.slack, message.SlackAttachment{
		Title:     "Evergreen Patch",
		TitleLink: data.URL,
		Text:      t.patch.Description,
		Color:     slackColor,
		Fields: []*message.SlackAttachmentField{
			{
				Title: "Time Taken",
				Value: makespan,
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

func (t *patchTriggers) getGithubContext(projectIdentifier string) (string, error) {
	parentPatch, err := patch.FindOneId(t.patch.Triggers.ParentPatch)
	if err != nil {
		return "", errors.Wrapf(err, "getting parent patch '%s'", t.patch.Triggers.ParentPatch)
	}
	if parentPatch == nil {
		return "", errors.Errorf("parent patch '%s' not found", t.patch.Triggers.ParentPatch)
	}
	return patch.GetGithubContextForChildPatch(projectIdentifier, parentPatch, t.patch)
}

func (t *patchTriggers) patchFamilyOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) {
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

func (t *patchTriggers) patchFamilySuccess(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !evergreen.IsFinishedVersionStatus(t.data.Status) || t.event.EventType != event.PatchChildrenCompletion {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchFamilyFailure(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed || t.event.EventType != event.PatchChildrenCompletion {
		return nil, nil
	}
	return t.generate(sub)
}
