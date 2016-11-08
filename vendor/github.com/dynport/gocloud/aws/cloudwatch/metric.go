package cloudwatch

import (
	"encoding/xml"
	"net/url"

	"github.com/dynport/gocloud/aws"
)

type Dimension struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

type Metric struct {
	Dimensions []*Dimension `xml:"Dimensions>member"`
	MetricName string       `xml:"MetricName"`
	Namespace  string       `xml:"Namespace"`
}

type ListMetricsResponse struct {
	XMLName   xml.Name  `xml:"ListMetricsResponse"`
	Metrics   []*Metric `xml:"ListMetricsResult>Metrics>member"`
	NextToken string    `xml:"ListMetricsResult>NextToken"`
}

const (
	VERSION = "2010-08-01"
)

func endpoint(client *aws.Client) string {
	return "https://monitoring." + client.Region + ".amazonaws.com"
}

type Client struct {
	*aws.Client
}

func (client *Client) Endpoint() string {
	prefix := "https://monitoring"
	if client.Client.Region != "" {
		prefix += "." + client.Client.Region
	}
	return prefix + ".amazonaws.com"
}

func (client *Client) ListMetrics() (rsp *ListMetricsResponse, e error) {
	values := &url.Values{}
	values.Add("Version", VERSION)
	values.Add("Action", "ListMetrics")
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return nil, e
	}
	e = xml.Unmarshal(raw.Content, &rsp)
	return rsp, e
}
