package s3

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

type PolicyRequest struct {
	Bucket string
	Key    string
}

type Policy struct {
	Id         string             `json:"Id,omitempty"`
	Version    string             `json:"Version,omitempty"`
	Statements []*PolicyStatement `json:"Statement,omitempty"`
}

type PolicyStatement struct {
	Condition interface{} `json:"Condition"`
	Resource  string      `json:"Resource,omitempty"`
	Action    string      `json:"Action,omitempty"`
	Principal interface{} `json:"Principal,omitempty"`
	Effect    string      `json:"Effect,omitempty"`
	Sid       string      `json:"Sid,omitempty"`
}

func (pr *PolicyRequest) Load(client *Client) (*AccessControlPolicy, error) {
	if pr.Bucket == "" {
		return nil, fmt.Errorf("Bucket must be set")
	}
	path := pr.Bucket
	if pr.Key != "" {
		path += "/" + pr.Key
	}
	u := "https://" + client.EndpointHost() + "/" + path + "?policy"
	req, e := http.NewRequest("GET", u, nil)
	client.SignS3Request(req, pr.Bucket)
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
