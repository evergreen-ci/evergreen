package annotations

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/db"
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

// AddIssueToAnnotation is used to add an issue via the UI
func AddIssueToAnnotation(taskId string, execution int, issue IssueLink, username string) error {
	return errors.Wrapf(addIssueToAnnotationByType(taskId, execution, issue, username, IssuesKey),
		"problem adding task annotation issue for task '%s'", taskId)
}

func AddSuspectedIssueToAnnotation(taskId string, execution int, issue IssueLink, username string) error {
	return errors.Wrapf(addIssueToAnnotationByType(taskId, execution, issue, username, SuspectedIssuesKey),
		"problem adding task annotation suspected issue for task '%s'", taskId)
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

func addIssueToAnnotationByType(taskId string, execution int, issue IssueLink, username, key string) error {
	issue.Source = &Source{
		Author:    username,
		Time:      time.Now(),
		Requester: UIRequester,
	}

	existingAnnotation, err := FindOneByTaskIdAndExecution(taskId, execution)
	if err != nil {
		return errors.Wrap(err, "error getting task annotation")
	}
	if existingAnnotation == nil {
		newAnnotation := newTaskAnnotation(taskId, execution)
		if key == IssuesKey {
			newAnnotation.Issues = []IssueLink{issue}
		} else {
			newAnnotation.SuspectedIssues = []IssueLink{issue}
		}
		return newAnnotation.Insert()
	}

	return db.UpdateId(Collection, existingAnnotation.Id, bson.M{
		"$push": bson.M{key: issue},
	})
}
