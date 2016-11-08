package cloudwatch

import (
	"encoding/xml"
	"log"
	"net/url"
	"os"
	"strconv"

	"github.com/dynport/gocloud/aws/ec2"
)

type ListMetrics struct {
	Dimensions []*Dimension
	MetricName string
	Namespace  string
	NextToken  string
}

type Values map[string]string

func (values Values) addDimensions(prefix string, dimensions []*Dimension) {
	for i, d := range dimensions {
		dimPrefix := prefix + "Dimensions.member." + strconv.Itoa(i+1) + "."
		values.Add(dimPrefix+"Name", d.Name)
		values.Add(dimPrefix+"Value", d.Value)
	}
}

func (values Values) Add(key, value string) {
	values[key] = value
}

func (values Values) Encode() string {
	ret := url.Values{}
	for k, v := range values {
		if v != "" {
			ret.Set(k, v)
		}
	}
	return ret.Encode()
}

var logger = log.New(os.Stderr, "", 0)

func (action *ListMetrics) Execute(client *ec2.Client) (*ListMetricsResponse, error) {
	rsp, e := client.DoSignedRequest("GET", endpoint(client.Client), action.query(), nil)
	if e != nil {
		return nil, e
	}
	o := &ListMetricsResponse{}
	e = xml.Unmarshal(rsp.Content, o)
	return o, e
}

func (action *ListMetrics) query() string {
	values := Values{
		"Version":    VERSION,
		"Action":     "ListMetrics",
		"MetricName": action.MetricName,
		"Namespace":  action.Namespace,
		"NextToken":  action.NextToken,
	}

	for i, d := range action.Dimensions {
		prefix := "Dimensions.member." + strconv.Itoa(i+1) + "."
		values.Add(prefix+"Name", d.Name)
		values.Add(prefix+"Value", d.Value)
	}
	return values.Encode()
}
