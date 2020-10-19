package model

import (
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
	Id            *string             `json:"id"`
	Note          *string             `json:"note"`
	Issues        []APIIssueLink      `json:"issues"`
	SuspectIssues []APIIssueLink      `json:"suspected_issues"`
	Source        APIAnnotationSource `json:"source"`
	Metadata      *string             `json:"metadata"`
}

// APIAnnotationSourceBuildFromService takes the task_annotations.AnnotationSource DB struct and
// returns the REST struct *APIAnnotationSource with the corresponding fields populated
func APIAnnotationSourceBuildFromService(t task_annotations.AnnotationSource) *APIAnnotationSource {
	m := APIAnnotationSource{}
	m.Author = StringStringPtr(t.Author)
	m.Time = TimeTimeTimeTimePtr(t.Time)
	return &m
}

// APIAnnotationSourceToService takes the APIAnnotationSource REST struct and returns the DB struct
// *task_annotations.AnnotationSource with the corresponding fields populated
func APIAnnotationSourceToService(m APIAnnotationSource) *task_annotations.AnnotationSource {
	out := &task_annotations.AnnotationSource{}
	out.Author = StringPtrString(m.Author)
	out.Time = TimeTimePtrTimeTime(m.Time)
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
	m.SuspectIssues = ArrtaskannotationsIssueLinkArrAPIIssueLink(t.SuspectIssues)
	m.Source = *APIAnnotationSourceBuildFromService(t.Source)
	m.Metadata = StringStringPtr(t.Metadata)
	return &m
}

// APITaskAnnotationToService takes the APITaskAnnotation REST struct and returns the DB struct
// *task_annotations.TaskAnnotation with the corresponding fields populated
func APITaskAnnotationToService(m APITaskAnnotation) *task_annotations.TaskAnnotation {
	out := &task_annotations.TaskAnnotation{}
	out.Id = StringPtrString(m.Id)
	out.Issues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.Issues)
	out.Note = StringPtrString(m.Note)
	out.SuspectIssues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.SuspectIssues)
	out.Source = *APIAnnotationSourceToService(m.Source)
	out.Metadata = StringPtrString(m.Metadata)
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

func TimeTimeTimeTime(in time.Time) time.Time {
	return time.Time(in)
}

func TimeTimeTimeTimePtr(in time.Time) *time.Time {
	out := time.Time(in)
	return &out
}

func TimeTimePtrTimeTime(in *time.Time) time.Time {
	var out time.Time
	if in == nil {
		return out
	}
	return time.Time(*in)
}

func TimeTimePtrTimeTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := time.Time(*in)
	return &out
}
