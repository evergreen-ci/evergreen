package send

import (
	"fmt"
	"math"
	"time"

	"github.com/montanaflynn/stats"
)

type benchResult struct {
	name        string
	trials      int
	duration    time.Duration
	raw         []result
	dataSize    int
	operations  int
	foundErrors *bool
}

func (r *benchResult) evergreenPerfFormat() ([]interface{}, error) {
	timings := r.timings()

	median, err := stats.Median(timings)
	if err != nil {
		return nil, err
	}

	min, err := stats.Min(timings)
	if err != nil {
		return nil, err
	}

	max, err := stats.Max(timings)
	if err != nil {
		return nil, err
	}

	out := []interface{}{
		map[string]interface{}{
			"name": r.name + "-throughput",
			"results": map[string]interface{}{
				"1": map[string]interface{}{
					"seconds":        r.roundedRuntime().Seconds(),
					"ops_per_second": r.getThroughput(median),
					"ops_per_second_values": []float64{
						r.getThroughput(min),
						r.getThroughput(max),
					},
				},
			},
		},
	}

	if r.dataSize > 0 {
		out = append(out, interface{}(map[string]interface{}{
			"name": r.name + "-MB-adjusted",
			"results": map[string]interface{}{
				"1": map[string]interface{}{
					"seconds":        r.roundedRuntime().Seconds(),
					"ops_per_second": r.adjustResults(median),
					"ops_per_second_values": []float64{
						r.adjustResults(min),
						r.adjustResults(max),
					},
				},
			},
		}))
	}

	return out, nil
}

func (r *benchResult) timings() []float64 {
	out := []float64{}
	for _, r := range r.raw {
		out = append(out, r.duration.Seconds())
	}
	return out
}

func (r *benchResult) totalDuration() time.Duration {
	var out time.Duration
	for _, trial := range r.raw {
		out += trial.duration
	}
	return out
}

func (r *benchResult) adjustResults(data float64) float64 { return bytesToMB(r.dataSize) / data }
func (r *benchResult) getThroughput(data float64) float64 { return float64(r.operations) / data }
func (r *benchResult) roundedRuntime() time.Duration      { return roundDurationMS(r.duration) }

func (r *benchResult) String() string {
	return fmt.Sprintf("name=%s, trials=%d, secs=%s", r.name, r.trials, r.duration)
}

func (r *benchResult) hasErrors() bool {
	if r.foundErrors == nil {
		var val bool
		for _, res := range r.raw {
			if res.err != nil {
				val = true
				break
			}
		}
		r.foundErrors = &val
	}

	return *r.foundErrors
}

func (r *benchResult) errReport() []string {
	errs := []string{}
	for _, res := range r.raw {
		if res.err != nil {
			errs = append(errs, res.err.Error())
		}
	}
	return errs
}

type result struct {
	duration   time.Duration
	iterations int
	err        error
}

func roundDurationMS(d time.Duration) time.Duration {
	return d / 1e6 * time.Millisecond
}

func bytesToMB(numBytes int) float64 {
	return float64(numBytes) * math.Pow(2, -20)
}
