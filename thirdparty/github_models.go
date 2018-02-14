package thirdparty

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

const githubTimeFormat = "2006-01-02T15:04:05Z"

type GithubTime time.Time

// UnmarshalJSON is a helper to fix parsing of Github's Timestamps
// Github time stamps look like this: 2018-02-07T19:54:26Z
// strftime format: %Y-%m-%dT%H:%M:%SZ
// Z indicates Zulu time (UTC)
func (t *GithubTime) UnmarshalJSON(b []byte) error {
	var timeStr string
	if err := json.Unmarshal(b, &timeStr); err != nil {
		return errors.Wrap(err, "expected JSON string while parsing time")
	}
	if len(timeStr) == 0 {
		return errors.New("expected JSON string while parsing time")
	}

	loc, err := time.LoadLocation("UTC")
	if err != nil {
		return errors.Wrap(err, "error loading UTC time location")
	}

	newTime, err := time.ParseInLocation(githubTimeFormat, timeStr, loc)
	if err != nil {
		return errors.Wrapf(err, "Error parsing time '%s' in UTC time zone", timeStr)
	}

	*t = GithubTime(newTime)

	return nil
}

func (t *GithubTime) MarshalJSON() ([]byte, error) {
	timeStr := t.Time().Format(githubTimeFormat)

	return []byte(fmt.Sprintf(`"%s"`, timeStr)), nil
}

func (t *GithubTime) Time() time.Time {
	return time.Time(*t)
}

// Github API response structs
type PatchSummary struct {
	Name      string
	Additions int
	Deletions int
}

type CommitEvent struct {
	URL       string
	SHA       string
	Commit    CommitDetails
	Author    AuthorDetails
	Committer AuthorDetails
	Parents   []Tree
	Stats     Stats
	Files     []File
}

type BranchEvent struct {
	Name     string
	Commit   GithubCommit
	Author   CommitAuthor
	Parents  []Parent
	URL      string
	Commiter AuthorDetails
	Links    Link
}

type GithubCommit struct {
	Url       string
	SHA       string
	Commit    CommitDetails
	Author    CommitAuthor
	Committer CommitAuthor
	Parents   []Parent
}

type GithubFile struct {
	Name     string
	Path     string
	SHA      string
	Size     int
	URL      string
	HtmlURL  string
	GitURL   string
	Type     string
	Content  string
	Encoding string
	Links    Link
}

type Link struct {
	Self string
	Git  string
	Html string
}

type Parent struct {
	Url string
	Sha string
}

type CommitDetails struct {
	URL       string
	Author    CommitAuthor
	Committer CommitAuthor
	Message   string
	Tree      Tree
}

type CommitAuthor struct {
	Name  string
	Email string
	Date  GithubTime
}

type AuthorDetails struct {
	Login      string
	Id         int
	AvatarURL  string
	GravatarId string
	URL        string
}

type Tree struct {
	URL string
	SHA string
}

type Stats struct {
	Additions int
	Deletions int
	Total     int
}

type File struct {
	FileName    string
	Additions   int
	Deletions   int
	Changes     int
	Status      string
	RawURL      string
	BlobURL     string
	ContentsURL string
	Patch       string
}

type GithubLoginUser struct {
	Login            string
	Id               int
	Company          string
	EmailAddress     string `json:"email"`
	Name             string
	OrganizationsURL string
}

func (u *GithubLoginUser) DisplayName() string {
	return u.Name
}

func (u *GithubLoginUser) Email() string {
	return u.EmailAddress
}

func (u *GithubLoginUser) Username() string {
	return u.Login
}

func (u *GithubLoginUser) IsNil() bool {
	return u == nil
}

type GithubAuthParameters struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Code         string `json:"code"`
	RedirectUri  string `json:"redirect_uri"`
	State        string `json:"state"`
}

type GithubOrganization struct {
	Login string `json:"login"`
	Url   string `json:"url"`
}

type GithubAuthResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type GitHubCompareResponse struct {
	Url             string          `json:"url"`
	HtmlUrl         string          `json:"html_url"`
	PermalinkUrl    string          `json:"permalink_url"`
	DiffUrl         string          `json:"diff_url"`
	PatchUrl        string          `json:"patch_url"`
	BaseCommit      CommitEvent     `json:"base_commit"`
	Author          AuthorDetails   `json:"author"`
	Committer       AuthorDetails   `json:"committer"`
	Parents         []Parent        `json:"parents"`
	MergeBaseCommit CommitEvent     `json:"merge_base_commit"`
	Files           File            `json:"file"`
	Commits         []CommitDetails `json:"commits"`
	TotalCommits    int             `json:"total_commits"`
	BehindBy        int             `json:"behind_by"`
	AheadBy         int             `json:"ahead_by"`
	Status          string          `json:"status"`
}
