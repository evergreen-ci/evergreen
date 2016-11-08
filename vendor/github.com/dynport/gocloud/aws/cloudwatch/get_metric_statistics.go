package cloudwatch

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"time"

	"github.com/dynport/gocloud/aws/ec2"
)

type GetMetricStatistics struct {
	Dimensions []*Dimension
	EndTime    time.Time
	MetricName string
	Namespace  string
	Period     int // min: 60, multiple of 60
	StartTime  time.Time
	Statistics []string // Average | Sum | SampleCount | Maximum | Minimum
	Unit       string
}

func (action *GetMetricStatistics) Execute(client *ec2.Client) (*GetMetricStatisticsResponse, error) {
	if action.StartTime.IsZero() {
		return nil, fmt.Errorf("StartTime must not be zero")
	}
	if action.EndTime.IsZero() {
		return nil, fmt.Errorf("EndTime must not be zero")
	}
	if action.Period == 0 {
		action.Period = 60
	}
	values := Values{
		"Version":    VERSION,
		"Action":     "GetMetricStatistics",
		"MetricName": action.MetricName,
		"Namespace":  action.Namespace,
		"Period":     strconv.Itoa(action.Period),
		"EndTime":    action.EndTime.UTC().Format(time.RFC3339),
		"StartTime":  action.StartTime.UTC().Format(time.RFC3339),
		"Unit":       action.Unit,
	}

	for i, s := range action.Statistics {
		values.Add("Statistics.member."+strconv.Itoa(i+1), s)
	}

	values.addDimensions("", action.Dimensions)

	rsp, e := client.DoSignedRequest("GET", endpoint(client.Client), values.Encode(), nil)
	if e != nil {
		return nil, e
	}

	i := &GetMetricStatisticsResponse{}
	e = xml.Unmarshal(rsp.Content, i)
	return i, e
}

func loadGet(client *ec2.Client, values Values, i interface{}) error {
	rsp, e := client.DoSignedRequest("GET", endpoint(client.Client), values.Encode(), nil)
	if e != nil {
		return e
	}
	return xml.Unmarshal(rsp.Content, i)
}

type GetMetricStatisticsResponse struct {
	XMLName                   xml.Name                   `xml:"GetMetricStatisticsResponse"`
	GetMetricStatisticsResult *GetMetricStatisticsResult `xml:"GetMetricStatisticsResult"`
}

type GetMetricStatisticsResult struct {
	XMLName    xml.Name     `xml:"GetMetricStatisticsResult"`
	Label      string       `xml:"Label"`
	Datapoints []*Datapoint `xml:"Datapoints>member"`
}

type Datapoint struct {
	Timestamp   time.Time `xml:"Timestamp,omitempty"`
	Unit        string    `xml:"Unit,omitempty"`
	Sum         float64   `xml:"Sum,omitempty"`
	Average     float64   `xml:"Average,omitempty"`
	Maximum     float64   `xml:"Maximum,omitempty"`
	Minimum     float64   `xml:"Minimum,omitempty"`
	SampleCount float64   `xml:"SampleCount,omitempty"`
}
