package cloudformation

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/dynport/gocloud/aws"
)

type Client struct {
	*aws.Client
}

var httpClient = newHttpClient()

func newHttpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
}

func NewFromEnv() *Client {
	return &Client{Client: aws.NewFromEnv()}
}

func (client *Client) Endpoint() string {
	prefix := "https://cloudformation"
	if client.Client.Region != "" {
		prefix += "." + client.Client.Region
	}
	return prefix + ".amazonaws.com"
}

func (client *Client) loadCloudFormationResource(action string, params Values, i interface{}) error {
	req, e := client.signedCloudFormationRequest(action, params)

	rsp, e := httpClient.Do(req)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	switch rsp.StatusCode {
	case 404:
		return ErrorNotFound
	case 200:
		if i != nil {
			return xml.Unmarshal(b, i)
		}
		return nil
	default:
		ersp := &ErrorResponse{}
		dbg.Printf("ERROR=%q", string(b))
		e = xml.Unmarshal(b, ersp)
		if e != nil {
			return fmt.Errorf("expected status 2xx but got %s (%s)", rsp.Status, string(b))

		}
		if strings.Contains(ersp.Error.Message, "does not exist") {
			return ErrorNotFound
		}
		return fmt.Errorf(ersp.Error.Message)
	}
}

var (
	ErrorNotFound = fmt.Errorf("Error not found")
)

func (client *Client) signedCloudFormationRequest(action string, values Values) (*http.Request, error) {
	values["Version"] = "2010-05-15"
	values["Action"] = action
	theUrl := client.Endpoint() + "?" + values.Encode()
	req, e := http.NewRequest("GET", theUrl, nil)
	if e != nil {
		return nil, e
	}
	client.SignAwsRequestV2(req, time.Now())
	return req, nil
}
