package rds

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValuesFor(t *testing.T) {
	Convey("Values For", t, func() {
		type action struct {
			String  string
			Bool    bool
			Integer int
			Int64   int64
			//Filters []*ec2.Filter
		}

		type test struct {
			Action   *action
			Expected string
		}
		tests := []*test{
			{Action: &action{String: "test"}, Expected: "String=test"},
			{Action: &action{Bool: true}, Expected: "Bool=true"},
			{Action: &action{Integer: 10}, Expected: "Integer=10"},
			{Action: &action{Int64: int64(64)}, Expected: "Int64=64"},
		}
		for _, test := range tests {
			v := url.Values{}
			out := []string{}
			e := loadValues(v, test.Action)
			So(e, ShouldBeNil)
			for k, v := range v {
				out = append(out, fmt.Sprintf("%s=%s", k, strings.Join(v, ",")))
			}
			sort.Strings(out)
			So(strings.Join(out, "&"), ShouldEqual, test.Expected)
		}
	})
}
