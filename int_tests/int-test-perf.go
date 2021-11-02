package main

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/apimodels"
)

func main() {
	ctx := context.Background()
	opts := apimodels.GetCedarPerfCountOptions{
		//BaseURL:   evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		BaseURL:   "cedar.mongodb.com",
		TaskID:    "sys_perf_5.0_linux_3_node_replSet_bestbuy_agg_d86a36630a4166c9ad01de9404da075f634d02b3_21_10_19_04_35_11",
		Execution: 0,
	}

	result, err := apimodels.CedarPerfResultsCount(ctx, opts)
	if err != nil {
		fmt.Println(err)
		return
	}
	if result.NumberOfResults == 0 {

		fmt.Println("result is 0")
	}
	fmt.Printf("%v \n", result.NumberOfResults)
}
