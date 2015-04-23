package cloudwatch

import (
	"fmt"
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/aws"
	"github.com/dynport/gocloud/aws/cloudwatch"
)

func Register(router *cli.Router) {
	router.RegisterFunc("aws/cloudwatch", cloudwatchList, "List Cloudwatch metrics")
}

func cloudwatchList() error {
	client := cloudwatch.Client{Client: aws.NewFromEnv()}
	rsp, e := client.ListMetrics()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, m := range rsp.Metrics {
		table.Add(m.Namespace, m.MetricName)
		for _, d := range m.Dimensions {
			table.Add("", d.Name, d.Value)
		}
	}
	fmt.Println(table)
	return nil
}
