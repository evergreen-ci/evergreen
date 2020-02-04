package scripting

import (
	"context"
	"time"
)

// TestOutcome reflects the task status.
type TestOutcome string

const (
	TestOutcomeSuccess = "success"
	TestOutcomeFailure = "failure"
	TestOutcomeTimeout = "timeout"
)

type TestOptions struct {
	Name    string        `bson:"name" json:"name" yaml:"name"`
	Args    []string      `bson:"args" json:"args" yaml:"args"`
	Pattern string        `bson:"pattern" json:"pattern" yaml:"pattern"`
	Timeout time.Duration `bson:"timeout" json:"timeout" yaml:"timeout"`
	Count   int           `bson:"count" json:"count" yaml:"count"`
}

// TestResults capture the data about the run of a specific test run.
type TestResult struct {
	Name     string        `bson:"name" json:"name" yaml:"name"`
	StartAt  time.Time     `bson:"start_at" json:"start_at" yaml:"start_at"`
	Duration time.Duration `bson:"duration" json:"duration" yaml:"duration"`
	Outcome  TestOutcome   `bson:"outcome" json:"outcome" yaml:"outcome"`
}

func (opt TestOptions) getResult(ctx context.Context, err error, startAt time.Time) TestResult {
	out := TestResult{
		Name:     opt.Name,
		StartAt:  startAt,
		Duration: time.Since(startAt),
		Outcome:  TestOutcomeSuccess,
	}

	if opt.Timeout > 0 && out.Duration > opt.Timeout {
		out.Outcome = TestOutcomeTimeout
		return out
	}

	if ctx.Err() != nil {
		out.Outcome = TestOutcomeTimeout
		return out
	}

	if err != nil {
		out.Outcome = TestOutcomeFailure
		return out
	}

	return out
}
