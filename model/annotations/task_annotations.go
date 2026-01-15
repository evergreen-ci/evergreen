package annotations

import (
	"context"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
	Text   string  `bson:"text" json:"text"`
	Source *Source `bson:"source,omitempty" json:"source,omitempty"`
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

func UpdateAnnotationNote(ctx context.Context, taskId string, execution int, originalMessage, newMessage, username string) error {
	note := Note{
		Message: newMessage,
		Source:  &Source{Requester: UIRequester, Author: username, Time: time.Now()},
	}

	annotation, err := FindOneByTaskIdAndExecution(ctx, taskId, execution)
	if err != nil {
		return errors.Wrap(err, "finding task annotation")
	}

	if annotation != nil && annotation.Note != nil && annotation.Note.Message != originalMessage {
		return errors.New("note is out of sync, please try again")
	}
	_, err = db.Upsert(
		ctx,
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$set": bson.M{NoteKey: note},
		},
	)
	return errors.Wrapf(err, "updating note for task '%s'", taskId)
}

// SetAnnotationMetadataLinks sets the metadata links for a task annotation.
func SetAnnotationMetadataLinks(ctx context.Context, taskId string, execution int, username string, metadataLinks ...MetadataLink) error {
	now := time.Now()
	for i := range metadataLinks {
		metadataLinks[i].Source = &Source{
			Author:    username,
			Time:      now,
			Requester: UIRequester,
		}
	}

	_, err := db.Upsert(
		ctx,
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$set": bson.M{MetadataLinksKey: metadataLinks},
		},
	)
	return errors.Wrapf(err, "setting task links for task '%s'", taskId)
}

func AddSuspectedIssueToAnnotation(ctx context.Context, taskId string, execution int, issue IssueLink, username string) error {
	issue.Source = &Source{
		Author:    username,
		Time:      time.Now(),
		Requester: UIRequester,
	}

	_, err := db.Upsert(
		ctx,
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$push": bson.M{SuspectedIssuesKey: issue},
		},
	)
	return errors.Wrapf(err, "adding task annotation suspected issue for task '%s'", taskId)
}

func RemoveSuspectedIssueFromAnnotation(ctx context.Context, taskId string, execution int, issue IssueLink) error {
	return db.Update(
		ctx,
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{"$pull": bson.M{SuspectedIssuesKey: issue}},
	)
}

// CreateAnnotationUpdate constructs a DB update to modify an existing task annotation.
func CreateAnnotationUpdate(annotation *TaskAnnotation, userDisplayName string) bson.M {
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
	if annotation.MetadataLinks != nil {
		for i := range annotation.MetadataLinks {
			annotation.MetadataLinks[i].Source = source
		}
		update[MetadataLinksKey] = bson.M{"$each": annotation.MetadataLinks}
	}
	return update
}

func AddCreatedTicket(ctx context.Context, taskId string, execution int, ticket IssueLink, userDisplayName string) error {
	source := &Source{
		Author:    userDisplayName,
		Time:      time.Now(),
		Requester: WebhookRequester,
	}
	ticket.Source = source
	_, err := db.Upsert(
		ctx,
		Collection,
		ByTaskIdAndExecution(taskId, execution),
		bson.M{
			"$push": bson.M{CreatedIssuesKey: ticket},
		},
	)
	return errors.Wrapf(err, "adding ticket to task '%s'", taskId)
}

// ValidateMetadataLinks will validate the given metadata links, ensuring that they are valid URLs,
// that their text is not too long, and that there are not more than MaxMetadataLinks links provided.
func ValidateMetadataLinks(links ...MetadataLink) error {
	catcher := grip.NewBasicCatcher()
	if len(links) > MaxMetadataLinks {
		catcher.Errorf("cannot have more than %d task links per annotation", MaxMetadataLinks)
	}
	for _, link := range links {
		catcher.Add(util.CheckURL(link.URL))
		if link.Text == "" {
			catcher.Errorf("link text cannot be empty")
		} else if len(link.Text) > MaxMetadataTextLength {
			catcher.Errorf("link text cannot exceed %d characters", MaxMetadataTextLength)
		}
	}
	return catcher.Resolve()
}
