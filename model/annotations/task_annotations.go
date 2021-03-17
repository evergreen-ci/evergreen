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
	// links to tickets created from the task using a custom web hook
	CreatedIssues []IssueLink `bson:"created_issues,omitempty" json:"created_issues,omitempty"`
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

func MoveIssueToSuspectedIssue(taskId string, taskExecution int, issue IssueLink, username string) error {
	newIssue := issue
	newIssue.Source = &Source{Requester: UIRequester, Author: username, Time: time.Now()}
	q := ByTaskIdAndExecution(taskId, taskExecution)
	q[bsonutil.GetDottedKeyName(IssuesKey, IssueLinkIssueKey)] = issue.IssueKey
	return db.Update(
		Collection,
		q,
		bson.M{
			"$pull": bson.M{IssuesKey: bson.M{IssueLinkIssueKey: issue.IssueKey}},
			"$push": bson.M{SuspectedIssuesKey: newIssue},
		},
	)
}

func MoveSuspectedIssueToIssue(taskId string, taskExecution int, issue IssueLink, username string) error {
	newIssue := issue
	newIssue.Source = &Source{Requester: UIRequester, Author: username, Time: time.Now()}
	q := ByTaskIdAndExecution(taskId, taskExecution)
	q[bsonutil.GetDottedKeyName(SuspectedIssuesKey, IssueLinkIssueKey)] = issue.IssueKey
	return db.Update(
		Collection,
		q,
		bson.M{
			"$pull": bson.M{SuspectedIssuesKey: bson.M{IssueLinkIssueKey: issue.IssueKey}},
			"$push": bson.M{IssuesKey: newIssue},
		},
	)
}

func UpdateAnnotationNote(taskId string, execution int, originalMessage, newMessage, username string) error {
	note := Note{
		Message: newMessage,
		Source:  &Source{Requester: UIRequester, Author: username, Time: time.Now()},
	}

	annotation, err := FindOneByTaskIdAndExecution(taskId, execution)
	if err != nil {
		return errors.Wrapf(err, "error finding task annotation")
	}

	if annotation != nil && annotation.Note != nil && annotation.Note.Message != originalMessage {
		return errors.New("note is out of sync, please try again")
	}
	_, err = db.Upsert(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$set": bson.M{NoteKey: note},
		},
	)
	return errors.Wrapf(err, "problem updating note for task '%s'", taskId)
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

func AddCreatedTicket(taskId string, execution int, ticket IssueLink, userDisplayName string) error {
	source := &Source{
		Author:    userDisplayName,
		Time:      time.Now(),
		Requester: WebhookRequester,
	}
	ticket.Source = source
	_, err := db.Upsert(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$push": bson.M{CreatedIssuesKey: ticket},
		},
	)
	return errors.Wrapf(err, "problem adding ticket to '%s'", taskId)
}
