package poplar

import (
	"fmt"
	"strings"
	"time"

	"github.com/mongodb/grip/message"
)

// BenchmarkResult contains data about the run of a specific test. The
// ArtifactPath is populated with a link to the intra-run data
// collected during the execution of the test.
type BenchmarkResult struct {
	Name         string        `bson:"name" json:"name" yaml:"name"`
	Runtime      time.Duration `bson:"duration" json:"duration" yaml:"duration"`
	Count        int           `bson:"count" json:"count" yaml:"count"`
	Iterations   int           `bson:"iterations" json:"iterations" yaml:"iterations"`
	Workload     bool          `bson:"workload" json:"workload" yaml:"workload"`
	Instances    int           `bson:"instances,omitempty" json:"instances,omitempty" yaml:"instances,omitempty"`
	ArtifactPath string        `bson:"path" json:"path" yaml:"path"`
	StartAt      time.Time     `bson:"start_at" json:"start_at" yaml:"start_at"`
	CompletedAt  time.Time     `bson:"compleated_at" json:"compleated_at" yaml:"compleated_at"`
	Error        error         `bson:"-" json:"-" yaml:"-"`
}

// Report returns a multi-line string format using the format of the
// verbose output of go test to communicate the test's outcome.
func (res *BenchmarkResult) Report() string {
	out := []string{
		"=== RUN", res.Name,
		"    --- REPORT: " + fmt.Sprintf("count=%d, iters=%d, runtime=%s", res.Count, res.Iterations, roundDurationMS(res.Runtime)),
	}

	if res.Error != nil {
		out = append(out,
			fmt.Sprintf("    --- ERRORS: %s", res.Error.Error()),
			fmt.Sprintf("--- FAIL: %s (%s)", res.Name, roundDurationMS(res.Runtime)))
	} else {
		out = append(out, fmt.Sprintf("--- PASS: %s (%s)", res.Name, roundDurationMS(res.Runtime)))
	}

	return strings.Join(out, "\n")
}

// Export converts a benchmark result into a test structure to support
// integration with cedar.
func (res *BenchmarkResult) Export() Test {
	t := Test{
		CreatedAt:   res.StartAt,
		CompletedAt: res.CompletedAt,
		Artifacts: []TestArtifact{
			{
				LocalFile:        res.ArtifactPath,
				CreatedAt:        res.CompletedAt,
				PayloadFTDC:      true,
				EventsRaw:        true,
				DataUncompressed: true,
			},
		},
		Info: TestInfo{
			TestName: res.Name,
			Tags:     []string{"poplar"},
			Arguments: map[string]int32{
				"count":      int32(res.Count),
				"iterations": int32(res.Iterations),
			},
		},
	}

	if res.Workload {
		t.Info.Tags = append(t.Info.Tags, "workload")
		t.Info.Arguments["instances"] = int32(res.Instances)
	}

	return t
}

// Composer produces a grip/message.Composer implementation that
// allows for easy logging of a results object. The message's string
// form is the same as Report, but also includes a structured raw format.
func (res *BenchmarkResult) Composer() message.Composer { return &resultComposer{Res: res} }

type resultComposer struct {
	Res          *BenchmarkResult `bson:"result" json:"result" yaml:"result"`
	message.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	hasLogged    bool
}

func (c *resultComposer) String() string { return c.Res.Report() }
func (c *resultComposer) Loggable() bool { return c.Res != nil && c.Res.Name != "" }
func (c *resultComposer) Raw() interface{} {
	if c.hasLogged {
		return c
	}

	if c.Res.Error != nil {
		_ = c.Annotate("error", c.Res.Error.Error()) // nolint: gosec
	}

	c.hasLogged = true

	return c
}

// BenchmarkResultGroup holds the result of a single suite of
// benchmarks, and provides several helper methods.
type BenchmarkResultGroup []BenchmarkResult

// Report returns an aggregated report for all results.
func (res BenchmarkResultGroup) Report() string {
	out := make([]string, len(res))

	for idx := range res {
		out[idx] = res[idx].Report()
	}

	return strings.Join(out, "\n")
}

// Composer returns a grip/message.Composer implementation that
// aggregates Composers from all of the results.
func (res BenchmarkResultGroup) Composer() message.Composer {
	msgs := make([]message.Composer, len(res))

	for idx, res := range res {
		msgs[idx] = res.Composer()
	}

	return message.NewGroupComposer(msgs)
}

// Export converts a group of test results into a slice of tests in
// preparation for uploading those results.
func (res BenchmarkResultGroup) Export() []Test {
	out := make([]Test, len(res))
	for idx, r := range res {
		out[idx] = r.Export()
	}
	return out
}
