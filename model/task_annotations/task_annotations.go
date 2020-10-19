package task_annotations

import (
	"time"
)

type TaskAnnotation struct {
	Id string `bson:"_id" json:"id"`
	// comment about the failure
	Note string `bson:"note,omitempty" json:"note,omitempty"`
	// links to tickets definitely related.
	Issues []IssueLink `bson:"issues,omitempty" json:"issue,omitempty"`
	// links to tickets possibly related
	SuspectIssues []IssueLink `bson:"suspected_issues,omitempty" json:"suspected_issues,omitempty"`
	// annotation attribution
	Source AnnotationSource `bson:"source,omitempty" json:"source,omitempty"`
	// structured data about the task (not displayed in the UI, but available in the API)
	Metadata string `bson:"metadata,omitempty" json:"metadata,omitempty"`
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
