package annotations

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
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
	// links to be displayed in the UI metadata sidebar
	MetadataLinks []MetadataLink `bson:"metadata_links,omitempty" json:"metadata_links,omitempty"`
}

// MetadataLink represents an arbitrary link to be associated with a task.
type MetadataLink struct {
	// URL to link to
	URL string `bson:"url" json:"url"`
	// Text to be displayed
	Text string `bson:"text" json:"text"`
}

type IssueLink struct {
	URL string `bson:"url" json:"url"`
	// Text to be displayed
	IssueKey        string  `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
	Source          *Source `bson:"source,omitempty" json:"source,omitempty"`
	ConfidenceScore float64 `bson:"confidence_score,omitempty" json:"confidence_score,omitempty"`
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
	TaskData   []TaskData     `bson:"task_data" json:"task_data"`
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

// AddMetadataLinkToAnnotation adds a task link to the annotation for the given task.
func AddMetadataLinkToAnnotation(taskId string, execution int, metadataLink MetadataLink) error {
	_, err := db.Upsert(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$push": bson.M{MetadataLinksKey: metadataLink},
		},
	)
	return errors.Wrapf(err, "updating task links for task '%s'", taskId)
}

// RemoveMetadataLinkFromAnnotation removes a task link from the annotation for the given task.
func RemoveMetadataLinkFromAnnotation(taskId string, execution int, metadataLink MetadataLink) error {
	return db.Update(
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{"$pull": bson.M{MetadataLinksKey: metadataLink}},
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
	if a.MetadataLinks != nil {
		update[MetadataLinksKey] = a.MetadataLinks
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
	return errors.Wrapf(err, "adding task annotation for task '%s'", a.TaskId)
}

// InsertManyAnnotations updates the source for a list of task annotations and their issues and then inserts them into the DB.
func InsertManyAnnotations(updates []TaskUpdate) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	for _, u := range updates {
		update := createAnnotationUpdate(&u.Annotation, "")
		ops := make([]mongo.WriteModel, len(u.TaskData))
		for idx := 0; idx < len(u.TaskData); idx++ {
			ops[idx] = mongo.NewUpdateOneModel().
				SetUpsert(true).
				SetFilter(ByTaskIdAndExecution(u.TaskData[idx].TaskId, u.TaskData[idx].Execution)).
				SetUpdate(bson.M{"$push": update})
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
	if annotation.Issues != nil {
		for i := range annotation.Issues {
			annotation.Issues[i].Source = source
		}
		update[IssuesKey] = bson.M{"$each": annotation.Issues}
	}
	if annotation.SuspectedIssues != nil {
		for i := range annotation.SuspectedIssues {
			annotation.SuspectedIssues[i].Source = source
		}
		update[SuspectedIssuesKey] = bson.M{"$each": annotation.SuspectedIssues}
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

	update := createAnnotationUpdate(a, userDisplayName)

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
		bson.M{
			"$push": bson.M{CreatedIssuesKey: ticket},
		},
	)
	return errors.Wrapf(err, "adding ticket to task '%s'", taskId)
}

// ValidateMetadataLinks will validate the given metadata links, ensuring that they are valid URLs, that they match Evergreen's approved
// CORS origins, and that they are not too long. It also ensures that there are not more than MaxMetadataLinks links provided.
func ValidateMetadataLinks(links ...MetadataLink) error {
	settings := evergreen.GetEnvironment().Settings()
	catcher := grip.NewBasicCatcher()
	if len(links) > MaxMetadataLinks {
		catcher.Errorf("cannot have more than %d task links per annotation", MaxMetadataLinks)
	}
	for _, link := range links {
		catcher.Add(util.CheckURL(link.URL))
		if !utility.StringMatchesAnyRegex(link.URL, settings.Ui.CORSOrigins) {
			catcher.Errorf("task link URL '%s' must match a CORS origin", link.URL)
		}
		if link.Text == "" {
			catcher.Errorf("link text cannot be empty")
		}
		if len(link.Text) > MaxMetadataTextLength {
			catcher.Errorf("link text cannot exceed %d characters", MaxMetadataTextLength)
		}
	}
	return catcher.Resolve()
}
