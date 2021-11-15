package main

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/apimodels"
)

func testURL(URL string) {
	ctx := context.Background()
	opts := apimodels.GetCedarPerfCountOptions{
		BaseURL: URL,
		TaskID:  "sys_perf_5.0_linux_3_node_replSet_bestbuy_agg_d86a36630a4166c9ad01de9404da075f634d02b3_21_10_19_04_35_11",
		//TaskID:    "mongodb_mongo_master_enterprise_rhel_80_64_bit_dynamic_required_concurrency_sharded_causal_consistency_2_enterprise_rhel_80_64_bit_dynamic_required_patch_d92ef9bf69fc6cdde29a210e3a1184b67d13908c_61894ad62a60ed7e22a1f1ee_21_11_08_16_05_44",
		Execution: 0,
	}
	if opts.BaseURL == "" {
		fmt.Println("no URL")
		return
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

func main() {
	testURL("cedar.mongodb.com")
	testURL("")
}
