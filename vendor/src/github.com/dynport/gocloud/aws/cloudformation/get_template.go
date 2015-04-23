package cloudformation

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/dynport/gocloud/aws"
)

type GetTemplate struct {
	StackName string
}

type GetTemplateResponse struct {
	XMLName           xml.Name           `xml:"GetTemplateResponse"`
	GetTemplateResult *GetTemplateResult `xml:"GetTemplateResult,omitempty"`
}

type GetTemplateResult struct {
	TemplateBody string `xml:"TemplateBody,omitempty"`
}

func endpoint(client *aws.Client) (string, error) {
	if client.Region == "" {
		return "", fmt.Errorf("Region must be set")
	}
	return "https://cloudformation." + client.Region + ".amazonaws.com", nil
}

func (t *GetTemplate) Execute(client *aws.Client) (*GetTemplateResponse, error) {
	v := Values{
		"Action":    "GetTemplate",
		"StackName": t.StackName,
		"Version":   "2010-05-15",
	}

	ep, e := endpoint(client)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequest("GET", ep+"?"+v.Encode(), nil)
	if e != nil {
		return nil, e
	}
	client.SignAwsRequestV2(req, time.Now())
	rsp, e := httpClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()

	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	xmlRsp := &GetTemplateResponse{}
	e = xml.Unmarshal(b, xmlRsp)
	if e != nil {
		e = fmt.Errorf(e.Error() + ": " + string(b) + " status=" + rsp.Status)
	}
	return xmlRsp, e
}
