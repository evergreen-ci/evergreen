package autoscaling

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func (client *Client) Endpoint() (string, error) {
	if client.Region == "" {
		return "", fmt.Errorf("Region must be set")
	}
	return "https://" + client.Region + ".autoscaling.amazonaws.com", nil
}

func (client *Client) Load(method string, query string, i interface{}) error {

	ep, e := client.Endpoint()
	if e != nil {
		return e
	}
	req, e := http.NewRequest(method, ep+query, nil)

	if e != nil {
		return e
	}
	client.SignAwsRequestV2(req, time.Now())
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	if rsp.Status[0] != '2' {
		return fmt.Errorf("expected status 2xx, got %s. %s", rsp.Status, string(b))
	}
	if i == nil {
		return nil
	}
	return xml.Unmarshal(b, i)
}
