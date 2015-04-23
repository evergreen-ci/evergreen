package cloudwatch

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/dynport/gocloud/aws"
)

// http://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/API_PutMetricData.html
type PutMetricData struct {
	Namespace  string
	MetricData []*MetricData
}

// http://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/API_MetricDatum.html
type MetricData struct {
	Dimensions      []*Dimension
	MetricName      string
	StatisticValues *StatisticValues
	Timestamp       time.Time
	Unit            string
	Value           float64
}

type StatisticValues struct {
	Maximum     float64
	Minimum     float64
	SampleCount float64
	Sum         float64
}

func (p *PutMetricData) Execute(client *aws.Client) error {
	q := p.query()
	theUrl := endpoint(client) + "?" + q
	req, e := http.NewRequest("POST", theUrl, nil)
	if e != nil {
		return e
	}
	now := time.Now()
	client.SignAwsRequestV2(req, now)
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	if rsp.Status[0] != '2' {
		return fmt.Errorf("expected status 2xx, got %s. %s", rsp.Status, string(b))
	}
	return nil
}

func (p *PutMetricData) query() string {
	values := Values{
		"Version":   VERSION,
		"Action":    "PutMetricData",
		"Namespace": p.Namespace,
	}

	for i, d := range p.MetricData {
		prefix := "MetricData.member." + strconv.Itoa(i+1) + "."
		values[prefix+"MetricName"] = d.MetricName
		values[prefix+"Unit"] = d.Unit
		values[prefix+"Value"] = strconv.FormatFloat(d.Value, 'E', 10, 64)
		if !d.Timestamp.IsZero() {
			values[prefix+"Timestamp"] = d.Timestamp.UTC().Format(time.RFC3339)
		}
		values.addDimensions(prefix, d.Dimensions)
	}
	return values.Encode()
}
