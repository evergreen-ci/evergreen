package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mongodb/amboy/benchmarks"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := benchmarks.RunQueue(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
