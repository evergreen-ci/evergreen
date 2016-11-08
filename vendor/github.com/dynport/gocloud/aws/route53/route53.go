package route53

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/dynport/gocloud/aws"
)

const API_VERSION = "2012-12-12"

func NewFromEnv() *Client {
	return &Client{
		aws.NewFromEnv(),
	}
}

// http://docs.aws.amazon.com/Route53/latest/APIReference/Welcome.html
type Client struct {
	*aws.Client
}

type HostedZone struct {
	Id                     string `xml:"Id"`
	Name                   string `xml:"Name"`
	CallerReference        string `xml:"CallerReference"`
	ResourceRecordSetCount int    `xml:"ResourceRecordSetCount"`
}

type ChangeResourceRecordSetsRequest struct {
	XMLName     xml.Name     `xml:"ChangeResourceRecordSetsRequest"`
	Xmlns       string       `xml:"xmlns,attr"`
	ChangeBatch *ChangeBatch `xml:"ChangeBatch"`
}

type ChangeBatch struct {
	XMLName xml.Name  `xml:"ChangeBatch"`
	Comment string    `xml:"Comment,omitempty"`
	Changes []*Change `xml:"Changes>Change"`
}

type Change struct {
	Action            string // CREATE or DELETE
	ResourceRecordSet *ResourceRecordSet
}

func (zone *HostedZone) Code() string {
	chunks := strings.Split(zone.Id, "/")
	if len(chunks) == 3 {
		return chunks[2]
	}
	return ""
}

type HttpResponse struct {
	StatusCode int
	Content    []byte
}

type ResourceRecord struct {
	Value string `xml:"Value"`
}

type ResourceRecordSet struct {
	Name            string            `xml:"Name"`
	Type            string            `xml:"Type"`
	TTL             int               `xml:"TTL"`
	HealthCheckId   string            `xml:"HealthCheckId,omitempty"`
	SetIdentifier   string            `xml:"SetIdentifier,omitempty"`
	Weight          int               `xml:"Weight,omitempty"`
	ResourceRecords []*ResourceRecord `xml:"ResourceRecords>ResourceRecord"`
}

type ListResourceRecordSetsResponse struct {
	XMLName            xml.Name             `xml:"ListResourceRecordSetsResponse"`
	ResourceRecordSets []*ResourceRecordSet `xml:"ResourceRecordSets>ResourceRecordSet"`
}

func NewChangeResourceRecordSets(batch *ChangeBatch) *ChangeResourceRecordSetsRequest {
	return &ChangeResourceRecordSetsRequest{
		Xmlns:       "https://route53.amazonaws.com/doc/" + API_VERSION + "/",
		ChangeBatch: batch,
	}
}

func (client *Client) ChangeResourceRecordSets(hostedZone string, changes []*Change) error {
	req := NewChangeResourceRecordSets(&ChangeBatch{
		Changes: changes,
	})
	buf := &bytes.Buffer{}
	e := xml.NewEncoder(buf).Encode(req)
	if e != nil {
		return e
	}
	httpRequest, e := http.NewRequest("POST", apiEndpoint+"/hostedzone/"+hostedZone+"/rrset", buf)
	if e != nil {
		return e
	}

	client.SignAwsRequest(httpRequest)

	rsp, e := http.DefaultClient.Do(httpRequest)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()

	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	if rsp.Status[0] != '2' {
		return fmt.Errorf("expected status 2xx, got %s (%s)", rsp.Status, string(b))
	}
	return nil
}

func (client *Client) ListResourceRecordSets(id string) (rrs []*ResourceRecordSet, e error) {
	raw, e := client.doRequest("GET", "hostedzone/"+id+"/rrset")
	if e != nil {
		return nil, e
	}
	rsp := &ListResourceRecordSetsResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	return rsp.ResourceRecordSets, e
}

func (client *Client) GetHostedZone(id string) (zone *HostedZone, e error) {
	rsp, e := client.doRequest("GET", "hostedzone/"+id)
	if e != nil {
		return nil, e
	}
	fmt.Println(string(rsp.Content))
	return nil, nil
}

type GetHostedZoneResponse struct {
	XMLName     xml.Name   `xml:"GetHostedZoneResponse"`
	HostedZone  HostedZone `xml:"HostedZone"`
	NameServers []string   `xml:"DelegationSet>NameServers>NameServer"`
}

func (client *Client) ListHostedZones() (zones []*HostedZone, e error) {
	rsp, e := client.doRequest("GET", "hostedzone")
	if e != nil {
		return zones, e
	}
	zonesResponse := &ListHostedZonesResponse{}
	e = xml.Unmarshal(rsp.Content, zonesResponse)
	if e != nil {
		return zones, fmt.Errorf("%q: %q", e, string(rsp.Content))
	}
	return zonesResponse.HostedZones, nil
}

type ListHostedZonesResponse struct {
	XMLName     xml.Name      `xml:"ListHostedZonesResponse"`
	IsTruncated bool          `xml:"IsTruncated"`
	MaxItems    int           `xml:"MaxItems"`
	HostedZones []*HostedZone `xml:"HostedZones>HostedZone"`
}

var apiEndpoint = "https://route53.amazonaws.com/" + API_VERSION

func (client *Client) doRequest(method, path string) (rsp *HttpResponse, e error) {
	request, e := http.NewRequest(method, apiEndpoint+"/"+path, nil)
	if e != nil {
		return nil, e
	}
	client.SignAwsRequest(request)
	raw, e := http.DefaultClient.Do(request)

	if e != nil {
		return nil, e
	}
	defer raw.Body.Close()
	rsp = &HttpResponse{
		StatusCode: raw.StatusCode,
	}
	rsp.Content, e = ioutil.ReadAll(raw.Body)
	return rsp, e
}
