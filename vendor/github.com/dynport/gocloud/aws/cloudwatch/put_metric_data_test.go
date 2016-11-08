package cloudwatch

import (
	"testing"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPutMetricData(t *testing.T) {
	Convey("Query", t, func() {
		So(1, ShouldEqual, 1)
		p := &PutMetricData{
			MetricData: []*MetricData{
				{
					Value: 10,
					Dimensions: []*Dimension{
						{Name: "InstanceId", Value: "test"},
					},
				},
			},
		}
		query := p.query()
		So(query, ShouldNotEqual, "")
		So(query, ShouldContainSubstring, "Value=1.0")
		So(query, ShouldNotContainSubstring, "Unit")
		So(query, ShouldContainSubstring, "MetricData.member.1.Dimensions.member.1.Name=InstanceId")
		t.Log(query)
	})

	Convey("List Metrics", t, func() {
		So((&ListMetrics{}).query(), ShouldNotEqual, "")
	})
}
