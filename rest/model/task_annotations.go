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
	Id *string `bson:"_id" json:"id"`
	// Identifier of the task that this annotation is for
	TaskId *string `bson:"task_id" json:"task_id"`
	// The number of the execution of the task that the annotation is for
	TaskExecution *int `bson:"task_execution" json:"task_execution"`
	// Structured data about the task. Since this is user-given json data, the structure can differ between annotations
	Metadata *birch.Document `bson:"metadata,omitempty" json:"metadata,omitempty" swaggertype:"object"`
	// Comment about the task failure
	Note *APINote `bson:"note,omitempty" json:"note,omitempty"`
	// Links to tickets definitely related
	Issues []APIIssueLink `bson:"issues,omitempty" json:"issues,omitempty"`
	// Links to tickets possibly related
	SuspectedIssues []APIIssueLink `bson:"suspected_issues,omitempty" json:"suspected_issues,omitempty"`
	CreatedIssues   []APIIssueLink `bson:"created_issues,omitempty" json:"created_issues,omitempty"`
	// List of links associated with a task, to be displayed in the task metadata sidebar, currently limited to 1
	MetadataLinks []APIMetadataLink `bson:"metadata_links,omitempty" json:"metadata_links,omitempty"`
}

type APINote struct {
	// Comment about the task failure
	Message *string `bson:"message,omitempty" json:"message,omitempty"`
	// The source of the note
	Source *APISource `bson:"source,omitempty" json:"source,omitempty"`
}
type APISource struct {
	// The author of the edit
	Author *string `bson:"author,omitempty" json:"author,omitempty"`
	// The time of the edit
	Time *time.Time `bson:"time,omitempty" json:"time,omitempty"`
	// The source of the request (api or ui)
	Requester *string `bson:"requester,omitempty" json:"requester,omitempty"`
}
type APIIssueLink struct {
	// The url of the ticket
	URL *string `bson:"url" json:"url"`
	// Text to be displayed
	IssueKey *string `bson:"issue_key,omitempty" json:"issue_key,omitempty"`
	// The source of the edit
	Source *APISource `bson:"source,omitempty" json:"source,omitempty"`
	// The confidence score of the issue
	ConfidenceScore *float64 `bson:"confidence_score,omitempty" json:"confidence_score,omitempty"`
}
type APIMetadataLink struct {
	// The url of the link
	URL *string `bson:"url" json:"url"`
	// Text to be displayed
	Text *string `bson:"text" json:"text"`
	// The source of the edit
	Source *APISource `bson:"source,omitempty" json:"source,omitempty"`
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
	m.Source = APISourceBuildFromService(t.Source)
	return &m
}

// APIMetadataLinkToService takes the APIMetadataLink REST struct and returns the DB struct
// *annotations.MetadataLink with the corresponding fields populated
func APIMetadataLinkToService(m APIMetadataLink) *annotations.MetadataLink {
	out := &annotations.MetadataLink{}
	out.URL = StringPtrString(m.URL)
	out.Text = StringPtrString(m.Text)
	out.Source = APISourceToService(m.Source)
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
	m.Issues = BuildAPIIssueLinks(t.Issues)
	m.SuspectedIssues = BuildAPIIssueLinks(t.SuspectedIssues)
	m.CreatedIssues = BuildAPIIssueLinks(t.CreatedIssues)
	m.MetadataLinks = BuildAPIMetadataLinks(t.MetadataLinks)
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
	out.Issues = BuildIssueLinks(m.Issues)
	out.SuspectedIssues = BuildIssueLinks(m.SuspectedIssues)
	out.CreatedIssues = BuildIssueLinks(m.CreatedIssues)
	apiMetadataLinks := []*APIMetadataLink{}
	for _, link := range m.MetadataLinks {
		apiMetadataLinks = append(apiMetadataLinks, &link)
	}
	out.MetadataLinks = APIMetadataLinksToService(apiMetadataLinks)
	out.Note = APINoteToService(m.Note)
	return out
}

// BuildAPIIssueLinks converts a slice of annotations.IssueLink to a slice of APIIssueLink
func BuildAPIIssueLinks(t []annotations.IssueLink) []APIIssueLink {
	if t == nil {
		return nil
	}
	m := []APIIssueLink{}
	for _, e := range t {
		m = append(m, *APIIssueLinkBuildFromService(e))
	}
	return m
}

// BuildIssueLinks converts a slice of APIIssueLink to a slice of annotations.IssueLink
func BuildIssueLinks(t []APIIssueLink) []annotations.IssueLink {
	if t == nil {
		return nil
	}
	m := []annotations.IssueLink{}
	for _, e := range t {
		m = append(m, *APIIssueLinkToService(e))
	}
	return m
}

// BuildAPIMetadataLinks converts a slice of annotations.MetadataLink to a slice of APIMetadataLink
func BuildAPIMetadataLinks(t []annotations.MetadataLink) []APIMetadataLink {
	if t == nil {
		return nil
	}
	m := []APIMetadataLink{}
	for _, e := range t {
		m = append(m, *APIMetadataLinkBuildFromService(e))
	}
	return m
}

// APIMetadataLinksToService converts a slice of APIMetadataLink to a slice of annotations.MetadataLink
func APIMetadataLinksToService(t []*APIMetadataLink) []annotations.MetadataLink {
	if t == nil {
		return nil
	}
	m := []annotations.MetadataLink{}
	for _, e := range t {
		m = append(m, *APIMetadataLinkToService(*e))
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
