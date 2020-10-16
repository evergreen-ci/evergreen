# go-tail

A go package for tailing files. `go get -u github.com/papertrail/go-tail`

## Usage

### File reading follower

```go
package main

import (
	"io"
	"fmt"
	"os"

	"github.com/papertrail/go-tail/follower"
)

func main() {
	t, err := follower.New("yourlogfile.log", follower.Config{
		Whence: io.SeekEnd,
		Offset: 0,
		Reopen: true,
	})

	for line := range t.Lines() {
		fmt.Println(line)
	}

	if t.Err() != nil {
		fmt.FPrintln(os.Stderr, t.Err())
	}
}

```
