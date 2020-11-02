package task_annotations

import (
	"time"

	"github.com/evergreen-ci/birch"
)

type TaskAnnotation struct {
	Id            string `bson:"_id" json:"id"`
	TaskId        string `bson:"task_id" json:"task_id"`
	TaskExecution int    `bson:"task_execution" json:"task_execution"`
	// the task annotation that is api sourced and will be overwritten when updated
	APIAnnotation Annotation `bson:"api_annotation,omitempty" json:"api_annotation,omitempty"`
	// user edits to the annotation that will be preserved
	UserAnnotation Annotation `bson:"user_annotation,omitempty" json:"user_annotation,omitempty"`
	// structured data about the task (not displayed in the UI, but available in the API)
	Metadata *birch.Document `bson:"metadata,omitempty" json:"metadata,omitempty"`
}

type IssueLink struct {
	URL string `bson:"url" json:"url"`
	// Text to be displayed
	IssueKey string `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
}

type AnnotationSource struct {
	Author string    `bson:"author,omitempty" json:"author,omitempty"`
	Time   time.Time `bson:"time,omitempty" json:"time,omitempty"`
}

type Annotation struct {
	// comment about the failure
	Note string `bson:"note,omitempty" json:"note,omitempty"`
	// links to tickets definitely related.
	Issues []IssueLink `bson:"issues,omitempty" json:"issues,omitempty"`
	// links to tickets possibly related
	SuspectedIssues []IssueLink `bson:"suspected_issues,omitempty" json:"suspected_issues,omitempty"`
	// annotation attribution
	Source AnnotationSource `bson:"source" json:"source"`
}
