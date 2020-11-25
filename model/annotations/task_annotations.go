package annotations

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type TaskAnnotation struct {
	Id            string          `bson:"_id" json:"id"`
	TaskId        string          `bson:"task_id" json:"task_id"`
	TaskExecution int             `bson:"task_execution" json:"task_execution"`
	Metadata      *birch.Document `bson:"metadata,omitempty" json:"metadata,omitempty"`
	// comment about the failure
	Note *Note `bson:"note,omitempty" json:"note,omitempty"`
	// links to tickets definitely related.
	Issues []IssueLink `bson:"issues,omitempty" json:"issues,omitempty"`
	// links to tickets possibly related
	SuspectedIssues []IssueLink `bson:"suspected_issues,omitempty" json:"suspected_issues,omitempty"`
}

type IssueLink struct {
	URL string `bson:"url" json:"url"`
	// Text to be displayed
	IssueKey string  `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
	Source   *Source `bson:"source,omitempty" json:"source,omitempty"`
}

type Source struct {
	Author    string    `bson:"author,omitempty" json:"author,omitempty"`
	Time      time.Time `bson:"time,omitempty" json:"time,omitempty"`
	Requester string    `bson:"requester,omitempty" json:"requester,omitempty"`
}

type Note struct {
	Message string  `bson:"message,omitempty" json:"message,omitempty"`
	Source  *Source `bson:"source,omitempty" json:"source,omitempty"`
}

func newTaskAnnotation(taskId string, execution int) TaskAnnotation {
	return TaskAnnotation{
		Id:            bson.NewObjectId().Hex(),
		TaskId:        taskId,
		TaskExecution: execution,
	}
}

// GetLatestExecutions returns only the latest execution for each task, and filters out earlier executions
func GetLatestExecutions(annotations []TaskAnnotation) []TaskAnnotation {
	highestExecutionAnnotations := map[string]TaskAnnotation{}
	for idx, a := range annotations {
		storedAnnotation, ok := highestExecutionAnnotations[a.TaskId]
		if !ok || storedAnnotation.TaskExecution < a.TaskExecution {
			highestExecutionAnnotations[a.TaskId] = annotations[idx]
		}
	}
	res := []TaskAnnotation{}
	for _, a := range highestExecutionAnnotations {
		res = append(res, a)
	}
	return res
}

func MoveIssueToSuspectedIssue(annotationId string, issue IssueLink, username string) error {
	newIssue := issue
	newIssue.Source = &Source{Requester: UIRequester, Author: username, Time: time.Now()}
	return db.Update(
		Collection,
		bson.M{
			IdKey: annotationId,
			bsonutil.GetDottedKeyName(IssuesKey, IssueLinkIssueKey): issue.IssueKey,
		},
		bson.M{
			"$pull": bson.M{IssuesKey: bson.M{IssueLinkIssueKey: issue.IssueKey}},
			"$push": bson.M{SuspectedIssuesKey: newIssue},
		},
	)
}

func MoveSuspectedIssueToIssue(annotationId string, issue IssueLink, username string) error {
	newIssue := issue
	newIssue.Source = &Source{Requester: UIRequester, Author: username, Time: time.Now()}
	return db.Update(
		Collection,
		bson.M{
			IdKey: annotationId,
			bsonutil.GetDottedKeyName(SuspectedIssuesKey, IssueLinkIssueKey): issue.IssueKey,
		},
		bson.M{
			"$pull": bson.M{SuspectedIssuesKey: bson.M{IssueLinkIssueKey: issue.IssueKey}},
			"$push": bson.M{IssuesKey: newIssue},
		},
	)
}

func AddIssueToAnnotation(taskId string, execution int, issue IssueLink, username string) error {
	issue.Source = &Source{
		Author:    username,
		Time:      time.Now(),
		Requester: UIRequester,
	}

	_, err := db.Upsert(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$push": bson.M{IssuesKey: issue},
		},
	)
	return errors.Wrapf(err, "problem adding task annotation issue for task '%s'", taskId)
}

func AddSuspectedIssueToAnnotation(taskId string, execution int, issue IssueLink, username string) error {
	issue.Source = &Source{
		Author:    username,
		Time:      time.Now(),
		Requester: UIRequester,
	}

	_, err := db.Upsert(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$push": bson.M{SuspectedIssuesKey: issue},
		},
	)
	return errors.Wrapf(err, "problem adding task annotation suspected issue for task '%s'", taskId)
}

func RemoveIssueFromAnnotation(taskId string, execution int, issue IssueLink) error {
	return db.Update(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{"$pull": bson.M{IssuesKey: issue}},
	)
}

func RemoveSuspectedIssueFromAnnotation(taskId string, execution int, issue IssueLink) error {
	return db.Update(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{"$pull": bson.M{SuspectedIssuesKey: issue}},
	)
}

func UpdateAnnotation(a *TaskAnnotation, userDisplayName string) error {
	source := &Source{
		Author:    userDisplayName,
		Time:      time.Now(),
		Requester: APIRequester,
	}
	update := bson.M{}

	if a.Metadata != nil {
		update[MetadataKey] = a.Metadata
	}
	if a.Note != nil {
		a.Note.Source = source
		update[NoteKey] = a.Note
	}
	if a.Issues != nil {
		for i := range a.Issues {
			a.Issues[i].Source = source
		}
		update[IssuesKey] = a.Issues
	}
	if a.SuspectedIssues != nil {
		for i := range a.SuspectedIssues {
			a.SuspectedIssues[i].Source = source
		}
		update[SuspectedIssuesKey] = a.SuspectedIssues
	}
	if len(update) == 0 {
		return nil
	}
	_, err := db.Upsert(
		Collection,
		ByTaskIdAndExecution(a.TaskId, a.TaskExecution),
		bson.M{
			"$set": update,
		},
	)
	return errors.Wrapf(err, "problem adding task annotation for '%s'", a.TaskId)
}
