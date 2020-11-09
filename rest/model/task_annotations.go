package model

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/model/task_annotations"
)

type APITaskAnnotation struct {
	Id             *string         `bson:"_id" json:"id"`
	TaskId         *string         `bson:"task_id" json:"task_id"`
	TaskExecution  int             `bson:"task_execution" json:"task_execution"`
	APIAnnotation  *APIAnnotation  `bson:"api_annotation,omitempty" json:"api_annotation,omitempty"`
	UserAnnotation *APIAnnotation  `bson:"user_annotation,omitempty" json:"user_annotation,omitempty"`
	Metadata       *birch.Document `bson:"metadata,omitempty" json:"metadata,omitempty"`
}
type APIAnnotation struct {
	Note          *APINote       `bson:"note,omitempty" json:"note,omitempty"`
	Issues        []APIIssueLink `bson:"issues,omitempty" json:"issues,omitempty"`
	SuspectIssues []APIIssueLink `bson:"suspected_issues,omitempty" json:"suspected_issues,omitempty"`
}
type APINote struct {
	Message *string    `bson:"message,omitempty" json:"message,omitempty"`
	Source  *APISource `bson:"note_source,omitempty" json:"note_source,omitempty"`
}
type APISource struct {
	Author *string    `bson:"author,omitempty" json:"author,omitempty"`
	Time   *time.Time `bson:"time,omitempty" json:"time,omitempty"`
}
type APIIssueLink struct {
	URL      *string   `bson:"url" json:"url"`
	IssueKey *string   `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
	Source   APISource `bson:"link_source,omitempty" json:"link_source,omitempty"`
}

// APIAnnotationBuildFromService takes the task_annotations.Annotation DB struct and
// returns the REST struct *APIAnnotation with the corresponding fields populated
func APIAnnotationBuildFromService(t *task_annotations.Annotation) *APIAnnotation {
	m := APIAnnotation{}
	m.Issues = ArrtaskannotationsIssueLinkArrAPIIssueLink(t.Issues)
	m.Note = APINoteBuildFromService(*t.Note)
	return &m
}

// APIAnnotationToService takes the APIAnnotation REST struct and returns the DB struct
// *task_annotations.Annotation with the corresponding fields populated
func APIAnnotationToService(m APIAnnotation) *task_annotations.Annotation {
	out := &task_annotations.Annotation{}
	out.Issues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.Issues)
	out.Note = APINoteToService(*m.Note)
	return out
}

// APISourceBuildFromService takes the task_annotations.Source DB struct and
// returns the REST struct *APISource with the corresponding fields populated
func APISourceBuildFromService(t task_annotations.Source) *APISource {
	m := APISource{}
	m.Author = StringStringPtr(t.Author)
	m.Time = ToTimePtr(t.Time)
	return &m
}

// APISourceToService takes the APISource REST struct and returns the DB struct
// *task_annotations.Source with the corresponding fields populated
func APISourceToService(m APISource) *task_annotations.Source {
	out := &task_annotations.Source{}
	out.Author = StringPtrString(m.Author)
	if m.Time != nil {
		out.Time = *m.Time
	} else {
		out.Time = time.Time{}
	}
	return out
}

// APIIssueLinkBuildFromService takes the task_annotations.IssueLink DB struct and
// returns the REST struct *APIIssueLink with the corresponding fields populated
func APIIssueLinkBuildFromService(t task_annotations.IssueLink) *APIIssueLink {
	m := APIIssueLink{}
	m.URL = StringStringPtr(t.URL)
	m.IssueKey = StringStringPtr(t.IssueKey)
	m.Source = *APISourceBuildFromService(t.Source)
	return &m
}

// APIIssueLinkToService takes the APIIssueLink REST struct and returns the DB struct
// *task_annotations.IssueLink with the corresponding fields populated
func APIIssueLinkToService(m APIIssueLink) *task_annotations.IssueLink {
	out := &task_annotations.IssueLink{}
	out.URL = StringPtrString(m.URL)
	out.IssueKey = StringPtrString(m.IssueKey)
	out.Source = *APISourceToService(m.Source)
	return out
}

// APINoteBuildFromService takes the task_annotations.Note DB struct and
// returns the REST struct *APINote with the corresponding fields populated
func APINoteBuildFromService(t task_annotations.Note) *APINote {
	m := APINote{}
	m.Message = StringStringPtr(t.Message)
	m.Source = APISourceBuildFromService(*t.Source)
	return &m
}

// APINoteToService takes the APINote REST struct and returns the DB struct
// *task_annotations.Note with the corresponding fields populated
func APINoteToService(m APINote) *task_annotations.Note {
	out := &task_annotations.Note{}
	out.Message = StringPtrString(m.Message)
	out.Source = APISourceToService(*m.Source)
	return out
}

// APITaskAnnotationBuildFromService takes the task_annotations.TaskAnnotation DB struct and
// returns the REST struct *APITaskAnnotation with the corresponding fields populated
func APITaskAnnotationBuildFromService(t task_annotations.TaskAnnotation) *APITaskAnnotation {
	m := APITaskAnnotation{}
	m.APIAnnotation = APIAnnotationBuildFromService(t.APIAnnotation)
	m.Id = StringStringPtr(t.Id)
	m.TaskExecution = t.TaskExecution
	m.TaskId = StringStringPtr(t.TaskId)
	m.UserAnnotation = APIAnnotationBuildFromService(t.UserAnnotation)
	m.Metadata = t.Metadata
	return &m
}

// APITaskAnnotationToService takes the APITaskAnnotation REST struct and returns the DB struct
// *task_annotations.TaskAnnotation with the corresponding fields populated
func APITaskAnnotationToService(m APITaskAnnotation) *task_annotations.TaskAnnotation {
	out := &task_annotations.TaskAnnotation{}
	out.APIAnnotation = APIAnnotationToService(*m.APIAnnotation)
	out.Id = StringPtrString(m.Id)
	out.TaskExecution = m.TaskExecution
	out.TaskId = StringPtrString(m.TaskId)
	out.UserAnnotation = APIAnnotationToService(*m.UserAnnotation)
	out.Metadata = m.Metadata
	return out
}

func ArrtaskannotationsIssueLinkArrAPIIssueLink(t []task_annotations.IssueLink) []APIIssueLink {
	m := []APIIssueLink{}
	for _, e := range t {
		m = append(m, *APIIssueLinkBuildFromService(e))
	}
	return m
}

func ArrAPIIssueLinkArrtaskannotationsIssueLink(t []APIIssueLink) []task_annotations.IssueLink {
	m := []task_annotations.IssueLink{}
	for _, e := range t {
		m = append(m, *APIIssueLinkToService(e))
	}
	return m
}
