package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

type AccessControlPolicy struct {
	XMLname           xml.Name           `xml:"AccessControlPolicy"`
	Owner             *User              `xml:"Owner"`
	AccessControlList *AccessControlList `xml:"AccessControlList"`
}

type AccessControlList struct {
	Grants []*Grant `xml:"Grant"`
}

type Grant struct {
	Grantee    *User  `xml:"Grantee"`
	Permission string `xml:"Permission"`
}

type User struct {
	ID          string `xml:"ID"`          // Owner-canonical-user-ID</ID>
	DisplayName string `xml:"DisplayName"` // display-name</DisplayName>
	URI         string `xml:"URI"`
}

type Acl struct {
	Bucket string
	Key    string
}

func (acl *Acl) Load(client *Client) (*AccessControlPolicy, error) {
	if acl.Bucket == "" {
		return nil, fmt.Errorf("Bucket must be set")
	}
	path := acl.Bucket
	if acl.Key != "" {
		path += "/" + acl.Key
	}
	u := "https://" + client.EndpointHost() + "/" + path + "?acl"
	req, e := http.NewRequest("GET", u, nil)
	client.SignS3Request(req, acl.Bucket)
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()

	buf := &bytes.Buffer{}

	r := io.TeeReader(rsp.Body, buf)

	acp := &AccessControlPolicy{}
	e = xml.NewDecoder(r).Decode(acp)
	return acp, e

}
