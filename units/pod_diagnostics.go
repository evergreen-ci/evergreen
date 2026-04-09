package units

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const podDiagnosticsJobName = "pod-diagnostics"

const (
	highMemoryThreshold     = 0.70
	highMemoryDuration      = time.Minute
	criticalMemoryThreshold = 0.90
)

var (
	podDiagnosticsMutex               sync.Mutex
	aboveHighMemoryThresholdSince     time.Time
	aboveCriticalMemoryThresholdSince time.Time
	memoryLastCheckedAt               time.Time
	profilingDataLastCollectedAt      time.Time
)

func init() {
	registry.AddJobType(podDiagnosticsJobName, func() amboy.Job {
		return makePodDiagnosticsJob()
	})
}

type podDiagnosticsJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makePodDiagnosticsJob() *podDiagnosticsJob {
	return &podDiagnosticsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podDiagnosticsJobName,
				Version: 0,
			},
		},
	}
}

// NewPodDiagnosticsJob returns a job that checks the current pod's memory
// usage against the configured thresholds and captures/stores profiling data in
// S3 when it's using excessive memory.
func NewPodDiagnosticsJob(ts string) amboy.Job {
	j := makePodDiagnosticsJob()
	j.SetID(fmt.Sprintf("%s.%s", podDiagnosticsJobName, ts))
	j.SetTimeInfo(amboy.JobTimeInfo{
		MaxTime: 5 * time.Minute,
	})
	return j
}

func (j *podDiagnosticsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.env.Settings().Diagnostics.S3BucketName == "" {
		return
	}

	// This lock is necessary to avoid multiple pod diagnostic jobs from running
	// concurrently. Typically, a remote queue job would use job scopes to
	// prevent concurrent operations. However, this job runs in the local queue
	// (since memory is per-app) and Evergreen's local queue implementation
	// (i.e. the limited size local queue) does not support scopes.
	podDiagnosticsMutex.Lock()
	defer podDiagnosticsMutex.Unlock()

	const minDurationBetweenMemoryChecks = 15 * time.Second
	if time.Since(memoryLastCheckedAt) < minDurationBetweenMemoryChecks {
		return
	}

	jobStartedAt := time.Now()

	usageBytes, limitBytes, err := j.getMemoryStats()
	if err != nil {
		j.AddError(errors.Wrap(err, "reading memory usage/limit for pod diagnostics"))
		return
	}

	memoryLastCheckedAt = jobStartedAt
	usagePercent := float64(usageBytes) / float64(limitBytes)

	if usagePercent >= criticalMemoryThreshold {
		if aboveCriticalMemoryThresholdSince.IsZero() {
			aboveCriticalMemoryThresholdSince = jobStartedAt
		}
		if aboveHighMemoryThresholdSince.IsZero() {
			aboveHighMemoryThresholdSince = jobStartedAt
		}
	} else if usagePercent >= highMemoryThreshold {
		if aboveHighMemoryThresholdSince.IsZero() {
			aboveHighMemoryThresholdSince = jobStartedAt
		}
		aboveCriticalMemoryThresholdSince = time.Time{}
	} else {
		aboveHighMemoryThresholdSince = time.Time{}
		aboveCriticalMemoryThresholdSince = time.Time{}
		profilingDataLastCollectedAt = time.Time{}
		return
	}

	const cooldownBetweenProfilingDataCollection = time.Minute
	if time.Since(profilingDataLastCollectedAt) < cooldownBetweenProfilingDataCollection {
		// Avoid gathering/uploading pprof too frequently during sustained high
		// memory usage.
		return
	}

	triggerReason := j.getTriggerReason(jobStartedAt, usagePercent)
	if triggerReason == "" {
		return
	}

	if err := j.collectAndUpload(ctx, usageBytes, limitBytes, usagePercent, triggerReason, jobStartedAt); err != nil {
		j.AddError(errors.Wrap(err, "collecting and uploading profiling data"))
		return
	}

	profilingDataLastCollectedAt = jobStartedAt
}

// getTriggerReason returns the reason that pprof is being triggered. Returns an
// empty string if pprof should not be triggered.
func (j *podDiagnosticsJob) getTriggerReason(jobStartedAt time.Time, usagePercent float64) string {
	if !aboveCriticalMemoryThresholdSince.IsZero() {
		return fmt.Sprintf(
			"critical memory usage - memory is at %.1f%% of container limit for %s (threshold: >%.1f%%)",
			usagePercent*100, jobStartedAt.Sub(aboveCriticalMemoryThresholdSince).String(), criticalMemoryThreshold*100,
		)
	}
	if !aboveHighMemoryThresholdSince.IsZero() && jobStartedAt.Sub(aboveHighMemoryThresholdSince) >= highMemoryDuration {
		return fmt.Sprintf(
			"high memory usage - memory is at %.1f%% of container limit for %s (threshold: >%.1f%% for >%s)",
			usagePercent*100, jobStartedAt.Sub(aboveHighMemoryThresholdSince).String(), highMemoryThreshold*100, highMemoryDuration.String(),
		)
	}
	return ""
}

func (j *podDiagnosticsJob) collectAndUpload(ctx context.Context, usageBytes, limitBytes int64, usagePercent float64, triggerReason string, ts time.Time) error {
	heapBuf, err := j.collectProfilingData("heap")
	if err != nil {
		return errors.Wrap(err, "collecting heap profile")
	}
	goroutineBuf, err := j.collectProfilingData("goroutine")
	if err != nil {
		return errors.Wrap(err, "collecting goroutine profile")
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	s3Prefix := fmt.Sprintf("%s/%s", ts.Format(TSFormat), hostname)
	diagnosticsConf := j.env.Settings().Diagnostics
	if err := j.uploadProfilingData(ctx, diagnosticsConf.S3BucketName, s3Prefix, heapBuf, goroutineBuf); err != nil {
		return errors.Wrap(err, "upload profiling data to S3")
	}

	const byteToGiB = 1 << 30
	grip.Notice(ctx, message.Fields{
		"message":          "successfully uploaded pod diagnostics for high memory usage",
		"trigger_reason":   triggerReason,
		"memory_usage_pct": fmt.Sprintf("%.1f%%", usagePercent*100),
		"memory_usage_gb":  fmt.Sprintf("%.2f", float64(usageBytes)/float64(byteToGiB)),
		"memory_limit_gb":  fmt.Sprintf("%.2f", float64(limitBytes)/float64(byteToGiB)),
		"hostname":         hostname,
		"s3_bucket":        diagnosticsConf.S3BucketName,
		"s3_prefix":        s3Prefix,
		"job":              j.ID(),
	})

	return nil
}

func (j *podDiagnosticsJob) collectProfilingData(name string) (*bytes.Buffer, error) {
	profile := pprof.Lookup(name)
	if profile == nil {
		return nil, nil
	}
	buf := &bytes.Buffer{}
	if err := profile.WriteTo(buf, 0); err != nil {
		return nil, errors.Wrapf(err, "collecting '%s' profile", name)
	}
	return buf, nil
}

// uploadProfilingData uploads heap and goroutine profiles to S3.
func (j *podDiagnosticsJob) uploadProfilingData(ctx context.Context, s3Bucket, s3Prefix string, heapBuf, goroutineBuf *bytes.Buffer) error {
	bucket, err := pail.NewS3Bucket(ctx, pail.S3Options{
		Name:   s3Bucket,
		Prefix: s3Prefix,
		Region: evergreen.DefaultEC2Region,
	})
	if err != nil {
		return errors.Wrap(err, "creating S3 bucket client")
	}

	catcher := grip.NewBasicCatcher()
	if heapBuf != nil && heapBuf.Len() > 0 {
		catcher.Wrap(bucket.Put(ctx, "heap", heapBuf), "uploading heap profile")
	}
	if goroutineBuf != nil && goroutineBuf.Len() > 0 {
		catcher.Wrap(bucket.Put(ctx, "goroutine", goroutineBuf), "uploading goroutine profile")
	}

	return catcher.Resolve()
}

// getMemoryStats returns the current memory usage and limit in bytes for the
// running container.
func (j *podDiagnosticsJob) getMemoryStats() (usageBytes, limitBytes int64, err error) {
	// Modern versions of Linux (such as what the app servers run on) support
	// cgroups v2. This system includes files that describe the current app
	// server's resource usage and limits.
	usage, err := j.readCgroupInt("/sys/fs/cgroup/memory.current")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading memory usage from cgroups v2")
	}
	limit, err := j.readCgroupInt("/sys/fs/cgroup/memory.max")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading memory limit from cgroups v2")
	}
	return usage, limit, nil
}

// readCgroupInt reads a single integer from a cgroup file.
func (j *podDiagnosticsJob) readCgroupInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, errors.Wrapf(err, "reading file '%s'", path)
	}
	// The cgroup files are assumed to contain a single string integer value (or
	// "max" if there's no limit).
	s := strings.TrimSpace(string(data))
	if s == "max" {
		return 0, errors.Errorf("file '%s' reports no limit (value: max)", path)
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parsing value '%s' from file '%s' as an integer", s, path)
	}
	return val, nil
}
