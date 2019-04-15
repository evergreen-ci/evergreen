package jasper

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
	dataSize    float64
	foundErrors *bool
}

func (r *benchResult) evergreenPerfFormat() ([]interface{}, error) {
	dataSizes := r.dataSizes()

	median, err := stats.Median(dataSizes)
	if err != nil {
		return nil, err
	}

	min, err := stats.Min(dataSizes)
	if err != nil {
		return nil, err
	}

	max, err := stats.Max(dataSizes)
	if err != nil {
		return nil, err
	}

	out := []interface{}{
		map[string]interface{}{
			"name": r.name + "-MB-adjusted",
			"results": map[string]interface{}{
				"1": map[string]interface{}{
					"seconds":        r.duration.Seconds(),
					"ops_per_second": r.adjustResults(median),
					"ops_per_second_values": []float64{
						r.adjustResults(min),
						r.adjustResults(max),
					},
				},
			},
		},
	}

	return out, nil
}

func (r *benchResult) dataSizes() []float64 {
	out := []float64{}
	for _, r := range r.raw {
		out = append(out, float64(r.size))
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

func (r *benchResult) totalSize() float64 {
	var out float64
	for _, trial := range r.raw {
		out += trial.size
	}
	return out
}

func (r *benchResult) adjustResults(data float64) float64 {
	return bytesToMB(data) / float64(r.duration.Seconds())
}

func (r *benchResult) String() string {
	return fmt.Sprintf("name=%s, trials=%d, size=%f, secs=%s", r.name, r.trials, bytesToMB(r.dataSize), r.duration)
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
	size     float64
	duration time.Duration
	err      error
}

func bytesToMB(numBytes float64) float64 {
	return numBytes * math.Pow(2.0, -20.0)
}
