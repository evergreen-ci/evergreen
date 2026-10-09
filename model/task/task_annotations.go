package task

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// MoveAnnotationIssueByKey moves an annotation issue between the issues and suspected issues
// lists by looking up the stored issue by its key.
// If isIssue is true, the issue is moved from issues to suspected issues; otherwise, from
// suspected issues to issues.
func MoveAnnotationIssueByKey(ctx context.Context, taskID string, taskExecution int, issueKey string, isIssue bool, username string) error {
	annotation, err := annotations.FindOneByTaskIdAndExecution(ctx, taskID, taskExecution)
	if err != nil {
		return errors.Wrapf(err, "finding annotation for execution %d of task '%s'", taskExecution, taskID)
	}
	if annotation == nil {
		return errors.Errorf("annotation for execution %d of task '%s' not found", taskExecution, taskID)
	}

	// Determine source and destination fields based on direction.
	var srcKey, destKey string
	var srcList []annotations.IssueLink
	if isIssue {
		srcKey = annotations.IssuesKey
		destKey = annotations.SuspectedIssuesKey
		srcList = annotation.Issues
	} else {
		srcKey = annotations.SuspectedIssuesKey
		destKey = annotations.IssuesKey
		srcList = annotation.SuspectedIssues
	}

	var found annotations.IssueLink
	for _, issue := range srcList {
		if issue.IssueKey == issueKey {
			found = issue
			break
		}
	}
	if found.IssueKey == "" {
		return errors.Errorf("issue with key '%s' not found in %s for execution %d of task '%s'", issueKey, srcKey, taskExecution, taskID)
	}
	found.Source = &annotations.Source{Requester: annotations.UIRequester, Author: username, Time: time.Now()}

	query := annotations.ByTaskIdAndExecution(taskID, taskExecution)
	query[bsonutil.GetDottedKeyName(srcKey, annotations.IssueLinkIssueKey)] = issueKey
	if err := db.Update(ctx, annotations.Collection, query, bson.M{
		"$pull": bson.M{srcKey: bson.M{annotations.IssueLinkIssueKey: issueKey}},
		"$push": bson.M{destKey: found},
	}); err != nil {
		return errors.Wrapf(err, "moving annotation issue for execution %d of task '%s'", taskExecution, taskID)
	}

	// We need to update the cached task hasAnnotation field if the move affects the issues array.
	hasIssues := !isIssue || len(annotation.Issues) > 1
	return UpdateHasAnnotations(ctx, taskID, taskExecution, hasIssues)
}

// AddIssueToAnnotation adds an issue onto an existing annotation and marks its associated task document
// as having annotations.
func AddIssueToAnnotation(ctx context.Context, taskId string, execution int, issue annotations.IssueLink, username string) error {
	issue.Source = &annotations.Source{
		Author:    username,
		Time:      time.Now(),
		Requester: annotations.UIRequester,
	}
	if _, err := db.Upsert(
		ctx,
		annotations.Collection,
		annotations.ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$push": bson.M{annotations.IssuesKey: issue},
		},
	); err != nil {
		return errors.Wrapf(err, "adding task annotation issue for task '%s'", taskId)
	}
	return UpdateHasAnnotations(ctx, taskId, execution, true)
}

// RemoveIssueFromAnnotation removes an issue from an existing annotation, and unsets its
// associated task document as having annotations if this was the last issue removed from the annotation.
func RemoveIssueFromAnnotation(ctx context.Context, taskId string, execution int, issue annotations.IssueLink) error {
	annotation := &annotations.TaskAnnotation{}
	_, err := db.FindAndModify(ctx,
		annotations.Collection,
		annotations.ByTaskIdAndExecution(taskId, execution),
		nil,
		adb.Change{
			Update:    bson.M{"$pull": bson.M{annotations.IssuesKey: issue}},
			ReturnNew: true,
		},
		annotation,
	)
	if err != nil {
		return errors.Wrapf(err, "finding and removing issue for task annotation for execution %d of task '%s'", execution, taskId)
	}
	if len(annotation.Issues) == 0 {
		return UpdateHasAnnotations(ctx, taskId, execution, false)
	}
	return nil
}

// UpsertAnnotation upserts a task annotation, and marks its associated task document
// as having annotations if the upsert includes a non-nil Issues field.
func UpsertAnnotation(ctx context.Context, a *annotations.TaskAnnotation, userDisplayName string) error {
	source := &annotations.Source{
		Author:    userDisplayName,
		Time:      time.Now(),
		Requester: annotations.APIRequester,
	}
	update := bson.M{}

	if a.Metadata != nil {
		update[annotations.MetadataKey] = a.Metadata
	}
	if a.Note != nil {
		a.Note.Source = source
		update[annotations.NoteKey] = a.Note
	}
	if a.MetadataLinks != nil {
		for i := range a.MetadataLinks {
			a.MetadataLinks[i].Source = source
		}
		update[annotations.MetadataLinksKey] = a.MetadataLinks
	}
	if a.Issues != nil {
		for i := range a.Issues {
			a.Issues[i].Source = source
		}
		update[annotations.IssuesKey] = a.Issues
	}
	if a.SuspectedIssues != nil {
		for i := range a.SuspectedIssues {
			a.SuspectedIssues[i].Source = source
		}
		update[annotations.SuspectedIssuesKey] = a.SuspectedIssues
	}
	if len(update) == 0 {
		return nil
	}
	if _, err := db.Upsert(
		ctx,
		annotations.Collection,
		annotations.ByTaskIdAndExecution(a.TaskId, a.TaskExecution),
		bson.M{
			"$set": update,
		},
	); err != nil {
		return errors.Wrapf(err, "adding task annotation for task '%s'", a.TaskId)
	}

	if a.Issues != nil {
		hasAnnotations := len(a.Issues) > 0
		return UpdateHasAnnotations(ctx, a.TaskId, a.TaskExecution, hasAnnotations)
	}

	return nil
}

// PatchAnnotation adds issues onto existing annotations, and marks its associated task document
// as having annotations if the patch includes a non-nil Issues field.
func PatchAnnotation(ctx context.Context, a *annotations.TaskAnnotation, userDisplayName string, upsert bool) error {
	existingAnnotation, err := annotations.FindOneByTaskIdAndExecution(ctx, a.TaskId, a.TaskExecution)
	if err != nil {
		return errors.Wrapf(err, "finding annotation for task '%s' and execution %d", a.TaskId, a.TaskExecution)
	}
	if existingAnnotation == nil {
		if !upsert {
			return errors.Errorf("annotation for task '%s' and execution %d not found", a.TaskId, a.TaskExecution)
		} else {
			return UpsertAnnotation(ctx, a, userDisplayName)
		}
	}

	if a.Note != nil {
		return errors.New("note should be empty")
	}
	if a.Metadata != nil {
		return errors.New("metadata should be empty")
	}

	update := annotations.CreateAnnotationUpdate(a, userDisplayName)

	if len(update) == 0 {
		return nil
	}

	if err = db.Update(
		ctx,
		annotations.Collection,
		annotations.ByTaskIdAndExecution(a.TaskId, a.TaskExecution),
		bson.M{
			"$push": update,
		},
	); err != nil {
		return errors.Wrapf(err, "updating task annotation for '%s'", a.TaskId)
	}

	if a.Issues != nil {
		hasAnnotations := len(a.Issues) > 0
		return UpdateHasAnnotations(ctx, a.TaskId, a.TaskExecution, hasAnnotations)
	}

	return nil
}
