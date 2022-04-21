package annotations

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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
	IssueKey        string  `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
	Source          *Source `bson:"source,omitempty" json:"source,omitempty"`
	ConfidenceScore float32 `bson:"confidence_score,omitempty" json:"confidence_score,omitempty"`
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

type TaskUpdate struct {
	Tasks      []TaskData     `bson:"tasks" json:"tasks"`
	Annotation TaskAnnotation `bson:"annotation" json:"annotation"`
}
type TaskData struct {
	TaskId    string `bson:"task_id" json:"task_id"`
	Execution int    `bson:"execution" json:"execution"`
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
		return errors.Wrap(err, "finding task annotation")
	}
	if annotation == nil {
		return errors.Errorf("annotation for task '%s' and execution %d not found", taskId, execution)
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
	return errors.Wrapf(err, "updating note for task '%s'", taskId)
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
	return errors.Wrapf(err, "adding task annotation issue for task '%s'", taskId)
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
	return errors.Wrapf(err, "adding task annotation suspected issue for task '%s'", taskId)
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
	update := createAnnotationUpdate(a, userDisplayName)
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
	return errors.Wrapf(err, "adding task annotation for task '%s'", a.TaskId)
}

// InsertManyAnnotations updates the source for a list of task annotations and their issues and then inserts them into the DB.
func InsertManyAnnotations(updates []TaskUpdate) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	for _, u := range updates {
		update := createAnnotationUpdate(&u.Annotation, "")
		ops := make([]mongo.WriteModel, len(u.Tasks))
		for idx := 0; idx < len(u.Tasks); idx++ {
			ops[idx] = mongo.NewUpdateOneModel().
				SetUpsert(true).
				SetFilter(ByTaskIdAndExecution(u.Tasks[idx].TaskId, u.Tasks[idx].Execution)).
				SetUpdate(bson.M{"$set": update})
		}
		_, err := env.DB().Collection(Collection).BulkWrite(ctx, ops, nil)

		if err != nil {
			return errors.Wrap(err, "bulk inserting annotations")
		}
	}
	return nil
}

func createAnnotationUpdate(annotation *TaskAnnotation, userDisplayName string) bson.M {
	source := &Source{
		Author:    userDisplayName,
		Time:      time.Now(),
		Requester: APIRequester,
	}
	update := bson.M{}
	if annotation.Metadata != nil {
		update[MetadataKey] = annotation.Metadata
	}
	if annotation.Note != nil {
		annotation.Note.Source = source
		update[NoteKey] = annotation.Note
	}
	if annotation.Issues != nil {
		for i := range annotation.Issues {
			annotation.Issues[i].Source = source
		}
		update[IssuesKey] = annotation.Issues
	}
	if annotation.SuspectedIssues != nil {
		for i := range annotation.SuspectedIssues {
			annotation.SuspectedIssues[i].Source = source
		}
		update[SuspectedIssuesKey] = annotation.SuspectedIssues
	}
	return update
}

// PatchAnnotation adds issues onto existing annotations.
func PatchAnnotation(a *TaskAnnotation, userDisplayName string, upsert bool) error {
	existingAnnotation, err := FindOneByTaskIdAndExecution(a.TaskId, a.TaskExecution)
	if err != nil {
		return errors.Wrapf(err, "finding annotation for task '%s' and execution %d", a.TaskId, a.TaskExecution)
	}
	if existingAnnotation == nil {
		if !upsert {
			return errors.Errorf("annotation for task '%s' and execution %d not found", a.TaskId, a.TaskExecution)
		} else {
			return UpdateAnnotation(a, userDisplayName)
		}
	}

	if a.Note != nil {
		return errors.New("note should be empty")
	}
	if a.Metadata != nil {
		return errors.New("metadata should be empty")
	}

	source := &Source{
		Author:    userDisplayName,
		Time:      time.Now(),
		Requester: APIRequester,
	}
	update := bson.M{}

	if a.Issues != nil {
		for i := range a.Issues {
			a.Issues[i].Source = source
		}
		update[IssuesKey] = bson.M{"$each": a.Issues}
	}

	if a.SuspectedIssues != nil {
		for i := range a.SuspectedIssues {
			a.SuspectedIssues[i].Source = source
		}
		update[SuspectedIssuesKey] = bson.M{"$each": a.SuspectedIssues}
	}

	if len(update) == 0 {
		return nil
	}

	err = db.Update(
		Collection,
		ByTaskIdAndExecution(a.TaskId, a.TaskExecution),
		bson.M{
			"$push": update,
		},
	)
	return errors.Wrapf(err, "updating task annotation for '%s'", a.TaskId)
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
		mgobson.M{
			"$push": mgobson.M{CreatedIssuesKey: ticket},
		},
	)
	return errors.Wrapf(err, "adding ticket to task '%s'", taskId)
}
