package jasper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type caseDefinition struct {
	name               string
	bench              func(context.Context, *caseDefinition) result
	requiredIterations int
	timeout            time.Duration
	procMaker          func(context.Context, *CreateOptions) (Process, error)
}

func (c *caseDefinition) run(ctx context.Context) *benchResult {
	out := &benchResult{
		name:     c.name,
		duration: c.timeout,
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 2*executionTimeout)
	defer cancel()

	fmt.Println("=== RUN", out.name)
	if c.requiredIterations == 0 {
		c.requiredIterations = minIterations
	}

benchRepeat:
	for {
		if ctx.Err() != nil {
			break
		}
		if out.trials >= c.requiredIterations {
			break
		}

		res := c.bench(ctx, c)

		switch res.err {
		case context.DeadlineExceeded:
			break benchRepeat
		case context.Canceled:
			break benchRepeat
		case nil:
			out.trials++
			out.raw = append(out.raw, res)
		default:
			grip.Error(errors.Wrap(res.err, "failed to run benchmark iteration"))
			continue
		}
	}

	out.dataSize = out.totalSize()
	fmt.Printf("    --- REPORT: trials=%d requiredTrials=%d timeout=%s\n", out.trials, c.requiredIterations, c.timeout)
	if out.hasErrors() {
		fmt.Printf("    --- ERRORS: %s\n", strings.Join(out.errReport(), "\n       "))
		fmt.Printf("--- FAIL: %s\n", out.name)
	} else {
		fmt.Printf("--- PASS: %s\n", out.name)
	}

	return out

}

func (c *caseDefinition) String() string {
	return fmt.Sprintf("name=%s, timeout=%s timeout=%s", c.name, c.timeout, executionTimeout)
}
