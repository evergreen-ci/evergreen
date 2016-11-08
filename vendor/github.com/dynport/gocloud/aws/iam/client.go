package iam

import (
	"encoding/xml"
	"github.com/dynport/gocloud/aws"
)

const (
	ENDPOINT    = "https://iam.amazonaws.com"
	API_VERSION = "2010-05-08"
)

type Client struct {
	*aws.Client
}

func NewFromEnv() *Client {
	return &Client{
		aws.NewFromEnv(),
	}
}

type GetUserResponse struct {
	User *User `xml:"GetUserResult>User"`
}

type Entry struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}

type SummaryMap struct {
	Entries []*Entry `xml:"entry"`
}

type GetAccountSummaryResponse struct {
	SummaryMap *SummaryMap `xml:"GetAccountSummaryResult>SummaryMap"`
}

type User struct {
	Path     string `xml:"Path"`
	UserName string `xml:"UserName"`
	UserId   string `xml:"UserId"`
	Arn      string `xml:"Arn"`
}

func (client *Client) GetUser(userName string) (user *User, e error) {
	raw, e := client.DoSignedRequest("GET", ENDPOINT, aws.QueryPrefix(API_VERSION, "GetUser"), nil)
	if e != nil {
		return user, e
	}
	rsp := &GetUserResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return user, e
	}
	return rsp.User, nil
}

func (client *Client) GetAccountSummary() (m *SummaryMap, e error) {
	raw, e := client.DoSignedRequest("GET", ENDPOINT, aws.QueryPrefix(API_VERSION, "GetAccountSummary"), nil)
	if e != nil {
		return m, e
	}
	rsp := &GetAccountSummaryResponse{}
	if e := aws.ExtractError(raw.Content); e != nil {
		return nil, e
	}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return m, e
	}
	return rsp.SummaryMap, nil
}

type ListUsersResponse struct {
	Users []*User `xml:"ListUsersResult>Users>member"`
}

func (client *Client) ListUsers() (users *ListUsersResponse, e error) {
	raw, e := client.DoSignedRequest("GET", ENDPOINT, aws.QueryPrefix(API_VERSION, "ListUsers"), nil)
	if e != nil {
		return users, e
	}
	rsp := &ListUsersResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return users, e
	}
	return rsp, nil
}

type ListAccountAliasesResponse struct {
	AccountAliases []string `xml:"ListAccountAliasesResult>AccountAliases>member"`
	IsTruncated    bool     `ListAccountAliasesResult>IsTruncated`
}

func (client *Client) ListAccountAliases() (aliases *ListAccountAliasesResponse, e error) {
	raw, e := client.DoSignedRequest("GET", ENDPOINT, aws.QueryPrefix(API_VERSION, "ListAccountAliases"), nil)
	if e != nil {
		return aliases, e
	}
	rsp := &ListAccountAliasesResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return rsp, e
	}
	return rsp, nil
}
