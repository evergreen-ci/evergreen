package model

import (
	"log"
	"net/url"
	"path"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

type APITaskAnnotation struct {
	Id              *string         `bson:"_id" json:"id"`
	TaskId          *string         `bson:"task_id" json:"task_id"`
	TaskExecution   *int            `bson:"task_execution" json:"task_execution"`
	Metadata        *birch.Document `bson:"metadata,omitempty" json:"metadata,omitempty"`
	Note            *APINote        `bson:"note,omitempty" json:"note,omitempty"`
	Issues          []APIIssueLink  `bson:"issues,omitempty" json:"issues,omitempty"`
	SuspectedIssues []APIIssueLink  `bson:"suspected_issues,omitempty" json:"suspected_issues,omitempty"`
	CreatedIssues   []APIIssueLink  `bson:"created_issues,omitempty" json:"created_issues,omitempty"`
}

type APINote struct {
	Message *string    `bson:"message,omitempty" json:"message,omitempty"`
	Source  *APISource `bson:"source,omitempty" json:"source,omitempty"`
}
type APISource struct {
	Author    *string    `bson:"author,omitempty" json:"author,omitempty"`
	Time      *time.Time `bson:"time,omitempty" json:"time,omitempty"`
	Requester *string    `bson:"requester,omitempty" json:"requester,omitempty"`
}
type APIIssueLink struct {
	URL      *string    `bson:"url" json:"url"`
	IssueKey *string    `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
	Source   *APISource `bson:"source,omitempty" json:"source,omitempty"`
}

// APISourceBuildFromService takes the annotations.Source DB struct and
// returns the REST struct *APISource with the corresponding fields populated
func APISourceBuildFromService(t *annotations.Source) *APISource {
	if t == nil {
		return nil
	}
	m := APISource{}
	m.Author = StringStringPtr(t.Author)
	m.Time = ToTimePtr(t.Time)
	m.Requester = StringStringPtr(t.Requester)
	return &m
}

// APISourceToService takes the APISource REST struct and returns the DB struct
// *annotations.Source with the corresponding fields populated
func APISourceToService(m *APISource) *annotations.Source {
	if m == nil {
		return nil
	}
	out := &annotations.Source{}
	out.Author = StringPtrString(m.Author)
	out.Requester = StringPtrString(m.Requester)
	if m.Time != nil {
		out.Time = *m.Time
	} else {
		out.Time = time.Time{}
	}
	return out
}

// APIIssueLinkBuildFromService takes the annotations.IssueLink DB struct and
// returns the REST struct *APIIssueLink with the corresponding fields populated
func APIIssueLinkBuildFromService(t annotations.IssueLink) *APIIssueLink {
	m := APIIssueLink{}
	m.URL = StringStringPtr(t.URL)
	m.IssueKey = StringStringPtr(t.IssueKey)
	m.Source = APISourceBuildFromService(t.Source)
	return &m
}

// APIIssueLinkToService takes the APIIssueLink REST struct and returns the DB struct
// *annotations.IssueLink with the corresponding fields populated
func APIIssueLinkToService(m APIIssueLink) *annotations.IssueLink {
	out := &annotations.IssueLink{}
	out.URL = StringPtrString(m.URL)
	out.IssueKey = StringPtrString(m.IssueKey)
	out.Source = APISourceToService(m.Source)
	return out
}

// APINoteBuildFromService takes the annotations.Note DB struct and
// returns the REST struct *APINote with the corresponding fields populated
func APINoteBuildFromService(t *annotations.Note) *APINote {
	if t == nil {
		return nil
	}
	m := APINote{}
	m.Message = StringStringPtr(t.Message)
	m.Source = APISourceBuildFromService(t.Source)
	return &m
}

// APINoteToService takes the APINote REST struct and returns the DB struct
// *annotations.Note with the corresponding fields populated
func APINoteToService(m *APINote) *annotations.Note {
	if m == nil {
		return nil
	}
	out := &annotations.Note{}
	out.Message = StringPtrString(m.Message)
	out.Source = APISourceToService(m.Source)
	return out
}

// APITaskAnnotationBuildFromService takes the annotations.TaskAnnotation DB struct and
// returns the REST struct *APITaskAnnotation with the corresponding fields populated
func APITaskAnnotationBuildFromService(t annotations.TaskAnnotation) *APITaskAnnotation {
	m := APITaskAnnotation{}
	m.Id = StringStringPtr(t.Id)
	m.TaskExecution = &t.TaskExecution
	m.TaskId = StringStringPtr(t.TaskId)
	m.Metadata = t.Metadata
	m.Issues = ArrtaskannotationsIssueLinkArrAPIIssueLink(t.Issues)
	m.SuspectedIssues = ArrtaskannotationsIssueLinkArrAPIIssueLink(t.SuspectedIssues)
	m.CreatedIssues = ArrtaskannotationsIssueLinkArrAPIIssueLink(t.CreatedIssues)
	m.Note = APINoteBuildFromService(t.Note)
	return &m
}

// APITaskAnnotationToService takes the APITaskAnnotation REST struct and returns the DB struct
// *annotations.TaskAnnotation with the corresponding fields populated
func APITaskAnnotationToService(m APITaskAnnotation) *annotations.TaskAnnotation {
	out := &annotations.TaskAnnotation{}
	out.Id = StringPtrString(m.Id)
	out.TaskExecution = *m.TaskExecution
	out.TaskId = StringPtrString(m.TaskId)
	out.Metadata = m.Metadata
	out.Issues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.Issues)
	out.SuspectedIssues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.SuspectedIssues)
	out.CreatedIssues = ArrAPIIssueLinkArrtaskannotationsIssueLink(m.CreatedIssues)
	out.Note = APINoteToService(m.Note)
	return out
}

func ArrtaskannotationsIssueLinkArrAPIIssueLink(t []annotations.IssueLink) []APIIssueLink {
	if t == nil {
		return nil
	}
	m := []APIIssueLink{}
	for _, e := range t {
		m = append(m, *APIIssueLinkBuildFromService(e))
	}
	return m
}

func ArrAPIIssueLinkArrtaskannotationsIssueLink(t []APIIssueLink) []annotations.IssueLink {
	if t == nil {
		return nil
	}
	m := []annotations.IssueLink{}
	for _, e := range t {
		m = append(m, *APIIssueLinkToService(e))
	}
	return m
}

func GetJiraTicketFromURL(URL string) (*thirdparty.JiraTicket, error) {
	settings := evergreen.GetEnvironment().Settings()
	jiraHandler := thirdparty.NewJiraHandler(*settings.Jira.Export())

	urlObject, err := url.Parse(URL)
	if err != nil {
		return nil, errors.Wrap(err, "problem parsing the issue url")
	}

	if urlObject != nil && urlObject.Host == "jira.mongodb.org" {
		myUrl, err := url.Parse(URL)
		if err != nil {
			log.Fatal(err)
		}
		jiraKey := path.Base(myUrl.Path)

		jiraTicket, err := jiraHandler.GetJIRATicket(jiraKey)
		if err != nil {
			return nil, errors.Wrap(err, "error getting Jira ticket")
		}
		return jiraTicket, nil
	}

	return nil, nil
}
