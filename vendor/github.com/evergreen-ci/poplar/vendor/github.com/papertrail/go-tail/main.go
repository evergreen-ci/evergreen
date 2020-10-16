package main

import (
	"fmt"
	"io"
	"os"

	"github.com/papertrail/go-tail/follower"
	flag "github.com/spf13/pflag"
)

var (
	number int64
	follow bool
	reopen bool
)

// "number" and "follow" are ignored for now, since the follower is the only behavior
// until reading the end of the file is implemented
func init() {
	flag.Int64VarP(&number, "number", "n", 10, "how many lines to return")
	flag.BoolVarP(&follow, "follow", "f", false, "follow file changes")
	flag.BoolVarP(&reopen, "reopen", "F", false, "implies -f and re-opens moved / truncated files")
}

func main() {
	flag.Parse()

	if reopen {
		follow = true
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "A file must be provided, I don't follow stdin right now")
		os.Exit(-1)
	}

	// printchan is an aggregate channel so that we can tail multiple files
	printChan := make(chan follower.Line)
	for _, f := range args {
		t, err := follower.New(f, follower.Config{
			Whence: io.SeekEnd,
			Offset: 0,
			Reopen: reopen,
		})

		if err != nil {
			panic(err)
		}

		go func(t *follower.Follower) {
			for l := range t.Lines() {
				printChan <- l
			}

			if err := t.Err(); err != nil {
				fmt.Println(err)
			}
		}(t)
	}

	for line := range printChan {
		fmt.Println(line.String())
	}
}
