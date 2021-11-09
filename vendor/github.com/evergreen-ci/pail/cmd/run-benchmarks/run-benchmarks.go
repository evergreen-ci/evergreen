package main

import (
	"context"
	"fmt"
	"os"

	"github.com/evergreen-ci/pail/benchmarks"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := benchmarks.RunSyncBucket(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
