package thirdparty

import (
	"time"
)

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
	Date  time.Time
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
