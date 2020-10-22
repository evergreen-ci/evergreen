package model

import (
	"encoding/json"
	"time"

	"github.com/evergreen-ci/evergreen/model/task_annotations"
)

type APIAnnotationSource struct {
	Author *string    `json:"author"`
	Time   *time.Time `json:"time"`
}
type APIIssueLink struct {
	URL      *string `json:"url"`
	IssueKey *string `json:"issue_key"`
}
type APITaskAnnotation struct {
	Id              *string             `json:"id"`
	Note            *string             `json:"note"`
	Issues          []APIIssueLink      `json:"issues"`
	SuspectedIssues []APIIssueLink      `json:"suspected_issues"`
	Source          APIAnnotationSource `json:"source"`
	Metadata        json.RawMessage     `json:"metadata"`
}

// APIAnnotationSourceBuildFromService takes the task_annotations.AnnotationSource DB struct and
// returns the REST struct *APIAnnotationSource with the corresponding fields populated
func APIAnnotationSourceBuildFromService(t task_annotations.AnnotationSource) *APIAnnotationSource {
	m := APIAnnotationSource{}
	m.Author = StringStringPtr(t.Author)
	m.Time = ToTimePtr(t.Time)
	return &m
}

// APIAnnotationSourceToService takes the APIAnnotationSource REST struct and returns the DB struct
// *task_annotations.AnnotationSource with the corresponding fields populated
func APIAnnotationSourceToService(m APIAnnotationSource) *task_annotations.AnnotationSource {
	out := &task_annotations.AnnotationSource{}
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
	return &m
}

// APIIssueLinkToService takes the APIIssueLink REST struct and returns the DB struct
// *task_annotations.IssueLink with the corresponding fields populated
func APIIssueLinkToService(m APIIssueLink) *task_annotations.IssueLink {
	out := &task_annotations.IssueLink{}
	out.URL = StringPtrString(m.URL)
	out.IssueKey = StringPtrString(m.IssueKey)
	return out
}

// APITaskAnnotationBuildFromService takes the task_annotations.TaskAnnotation DB struct and
// returns the REST struct *APITaskAnnotation with the corresponding fields populated
func APITaskAnnotationBuildFromService(t task_annotations.TaskAnnotation) *APITaskAnnotation {
	m := APITaskAnnotation{}
	m.Id = StringStringPtr(t.Id)
	m.Issues = ArrtaskannotationsIssueLinkArrAPIIssueLink(t.Issues)
	m.Note = StringStringPtr(t.Note)
	m.SuspectedIssues = ArrtaskannotationsIssueLinkArrAPIIssueLink(t.SuspectedIssues)
	m.Source = *APIAnnotationSourceBuildFromService(t.Source)
	m.Metadata = t.Metadata
	return &m
}

// APITaskAnnotationToService takes the APITaskAnnotation REST struct and returns the DB struct
// *task_annotations.TaskAnnotation with the corresponding fields populated
func APITaskAnnotationToService(m APITaskAnnotation) *task_annotations.TaskAnnotation {
	out := &task_annotations.TaskAnnotation{}
	out.Id = StringPtrString(m.Id)
	out.Issues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.Issues)
	out.Note = StringPtrString(m.Note)
	out.SuspectedIssues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.SuspectedIssues)
	out.Source = *APIAnnotationSourceToService(m.Source)
	out.Metadata = m.Metadata
	return out
}

//generated helper functions
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
