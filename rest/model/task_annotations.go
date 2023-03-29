package model

import (
	"net/url"
	"path"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type APITaskAnnotation struct {
	Id              *string           `bson:"_id" json:"id"`
	TaskId          *string           `bson:"task_id" json:"task_id"`
	TaskExecution   *int              `bson:"task_execution" json:"task_execution"`
	Metadata        *birch.Document   `bson:"metadata,omitempty" json:"metadata,omitempty"`
	Note            *APINote          `bson:"note,omitempty" json:"note,omitempty"`
	Issues          []APIIssueLink    `bson:"issues,omitempty" json:"issues,omitempty"`
	SuspectedIssues []APIIssueLink    `bson:"suspected_issues,omitempty" json:"suspected_issues,omitempty"`
	CreatedIssues   []APIIssueLink    `bson:"created_issues,omitempty" json:"created_issues,omitempty"`
	MetadataLinks   []APIMetadataLink `bson:"metadata_links,omitempty" json:"metadata_links,omitempty"`
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
	URL             *string    `bson:"url" json:"url"`
	IssueKey        *string    `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
	Source          *APISource `bson:"source,omitempty" json:"source,omitempty"`
	ConfidenceScore *float64   `bson:"confidence_score,omitempty" json:"confidence_score,omitempty"`
}
type APIMetadataLink struct {
	URL  *string `bson:"url" json:"url"`
	Text *string `bson:"text" json:"text"`
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

// APIMetadataLinkBuildFromService takes the annotations.MetadataLink DB struct and
// returns the REST struct *APIIssueLink with the corresponding fields populated
func APIMetadataLinkBuildFromService(t annotations.MetadataLink) *APIMetadataLink {
	m := APIMetadataLink{}
	m.URL = StringStringPtr(t.URL)
	m.Text = StringStringPtr(t.Text)
	return &m
}

// APIMetadataLinkToService takes the APIMetadataLink REST struct and returns the DB struct
// *annotations.MetadataLink with the corresponding fields populated
func APIMetadataLinkToService(m APIMetadataLink) *annotations.MetadataLink {
	out := &annotations.MetadataLink{}
	out.URL = StringPtrString(m.URL)
	out.Text = StringPtrString(m.Text)
	return out
}

// APIIssueLinkBuildFromService takes the annotations.IssueLink DB struct and
// returns the REST struct *APIIssueLink with the corresponding fields populated
func APIIssueLinkBuildFromService(t annotations.IssueLink) *APIIssueLink {
	m := APIIssueLink{}
	m.URL = StringStringPtr(t.URL)
	m.IssueKey = StringStringPtr(t.IssueKey)
	m.Source = APISourceBuildFromService(t.Source)
	m.ConfidenceScore = utility.ToFloat64Ptr(t.ConfidenceScore)
	return &m
}

// APIIssueLinkToService takes the APIIssueLink REST struct and returns the DB struct
// *annotations.IssueLink with the corresponding fields populated
func APIIssueLinkToService(m APIIssueLink) *annotations.IssueLink {
	out := &annotations.IssueLink{}
	out.URL = StringPtrString(m.URL)
	out.IssueKey = StringPtrString(m.IssueKey)
	out.Source = APISourceToService(m.Source)
	out.ConfidenceScore = utility.FromFloat64Ptr(m.ConfidenceScore)
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
	m.MetadataLinks = ArrtaskannotationsMetadataLinkArrAPIMetadataLink(t.MetadataLinks)
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
	out.MetadataLinks = ArrAPIMetadataLinkArrtaskannotationsMetadataLink(m.MetadataLinks)
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

func ArrtaskannotationsMetadataLinkArrAPIMetadataLink(t []annotations.MetadataLink) []APIMetadataLink {
	if t == nil {
		return nil
	}
	m := []APIMetadataLink{}
	for _, e := range t {
		m = append(m, *APIMetadataLinkBuildFromService(e))
	}
	return m
}

func ArrAPIMetadataLinkArrtaskannotationsMetadataLink(t []APIMetadataLink) []annotations.MetadataLink {
	if t == nil {
		return nil
	}
	m := []annotations.MetadataLink{}
	for _, e := range t {
		m = append(m, *APIMetadataLinkToService(e))
	}
	return m
}

func GetJiraTicketFromURL(jiraURL string) (*thirdparty.JiraTicket, error) {
	settings := evergreen.GetEnvironment().Settings()
	jiraHandler := thirdparty.NewJiraHandler(*settings.Jira.Export())

	parsedURL, err := url.Parse(jiraURL)
	if err != nil {
		return nil, errors.Wrap(err, "parsing Jira issue URL")
	}

	if parsedURL.Host == "jira.mongodb.org" {
		jiraKey := path.Base(parsedURL.Path)

		jiraTicket, err := jiraHandler.GetJIRATicket(jiraKey)
		if err != nil {
			return nil, errors.Wrapf(err, "getting Jira ticket for key '%s'", jiraKey)
		}
		return jiraTicket, nil
	}

	return nil, nil
}

func ValidateIssues(issues []APIIssueLink) error {
	catcher := grip.NewBasicCatcher()
	for _, issue := range issues {
		catcher.Add(util.CheckURL(utility.FromStringPtr(issue.URL)))
		score := utility.FromFloat64Ptr(issue.ConfidenceScore)
		if score < 0 || score > 100 {
			catcher.Errorf("confidence score '%f' must be between 0 and 100", score)
		}
	}
	return catcher.Resolve()
}
