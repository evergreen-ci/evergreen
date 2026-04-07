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
	criticalMemoryDuration  = 5 * time.Second
)

// podDiagnosticsState tracks threshold crossing durations across job runs.
// kim: TODO: see if this can be not a global variable, or otherwise named
// better.
var (
	podDiagnosticsMutex               sync.Mutex
	aboveHighMemoryThresholdSince     time.Time
	aboveCriticalMemoryThresholdSince time.Time
	memoryLastCheckedAt               time.Time
	pprofLastCollectedAt              time.Time
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
// usage against the configured thresholds and captures pprof diagnostic data
// when a threshold condition is met.
func NewPodDiagnosticsJob(ts string) amboy.Job {
	j := makePodDiagnosticsJob()
	j.SetID(fmt.Sprintf("%s.%s", podDiagnosticsJobName, ts))
	j.SetTimeInfo(amboy.JobTimeInfo{
		MaxTime: time.Minute,
	})
	return j
}

func (j *podDiagnosticsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.env.Settings().Diagnostics.S3BucketName == "" {
		// The diagnostics need to be uploaded to a bucket.
		grip.Info(ctx, message.Fields{
			"message": "S3 bucket not set, will not collect memory or pprof data",
			"job":     j.ID(),
		})
		return
	}

	// Note: this lock is necessary to avoid multiple pod diagnostic jobs from
	// running concurrently. Typically, most remote queue jobs would use scopes
	// to prevent concurrent operations. However, this job runs in the local
	// queue (since memory is per-app) and the local queue implementation that
	// Evergreen has historically used does not support scopes.
	podDiagnosticsMutex.Lock()
	defer podDiagnosticsMutex.Unlock()

	const minDurationBetweenMemoryChecks = 30 * time.Second
	if memoryLastCheckedAt.IsZero() || time.Since(memoryLastCheckedAt) < minDurationBetweenMemoryChecks {
		// Only check the memory periodically.
		return
	}

	jobStartedAt := time.Now()

	usageBytes, limitBytes, err := readCgroupMemory()
	if err != nil {
		j.AddError(errors.Wrap(err, "reading cgroup memory usage/limit for pod diagnostics"))
		return
	}
	memoryLastCheckedAt = jobStartedAt
	if limitBytes == 0 {
		// If the limit is zero, then the container has no memory limit
		// configured.
		// kim: TODO: confirm whether this actually happens in practice. May be
		// excessive/unnecessary guarding.
		return
	}

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
		pprofLastCollectedAt = time.Time{}
		return
	}

	// Avoid uploading pprof too frequently during sustained high memory usage.
	const cooldownBetweenPprofCollections = time.Minute
	if !pprofLastCollectedAt.IsZero() || time.Since(pprofLastCollectedAt) < cooldownBetweenPprofCollections {
		return
	}

	triggerReason := j.getTriggerReason(jobStartedAt, usagePercent)
	if triggerReason == "" {
		return
	}

	// Record that we collected pprof data already to avoid repeatedly
	// collecting pprof on every job run during sustained high memory usage.
	pprofLastCollectedAt = jobStartedAt

	// Collect and upload outside the lock (state is already reset above).
	// kim: TODO: double-check if we can do this outside the lock.
	j.collectAndUpload(ctx, usageBytes, limitBytes, usagePercent, triggerReason, jobStartedAt)
}

// getTriggerReason returns the reason that pprof is being triggered. Returns an
// empty string if pprof should not be triggered.
func (j *podDiagnosticsJob) getTriggerReason(jobStartedAt time.Time, usagePercent float64) string {
	if !aboveCriticalMemoryThresholdSince.IsZero() && jobStartedAt.Sub(aboveCriticalMemoryThresholdSince) >= criticalMemoryDuration {
		return fmt.Sprintf(
			"memory at %.1f%% of container limit for %.0fs (threshold: >90%% for >5s)",
			usagePercent*100, jobStartedAt.Sub(aboveCriticalMemoryThresholdSince).Seconds())
	}
	if !aboveHighMemoryThresholdSince.IsZero() && jobStartedAt.Sub(aboveHighMemoryThresholdSince) >= highMemoryDuration {
		return fmt.Sprintf(
			"memory at %.1f%% of container limit for %.0fs (threshold: >70%% for >60s)",
			usagePercent*100, jobStartedAt.Sub(aboveHighMemoryThresholdSince).Seconds())
	}
	return ""
}

func (j *podDiagnosticsJob) collectAndUpload(ctx context.Context, usageBytes, limitBytes int64, usagePercent float64, triggerReason string, ts time.Time) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	s3Prefix := fmt.Sprintf("%s/%s", ts.Format(TSFormat), hostname)

	heapBuf := &bytes.Buffer{}
	if err := pprof.WriteHeapProfile(heapBuf); err != nil {
		return errors.Wrap(err, "collecting heap profile")
	}

	goroutineBuf := &bytes.Buffer{}
	if profile := pprof.Lookup("goroutine"); profile != nil {
		if err := profile.WriteTo(goroutineBuf, 0); err != nil {
			return errors.Wrap(err, "collecting goroutine profile")
		}
	}

	diagnosticsConf := j.env.Settings().Diagnostics

	if err := j.uploadPprofFiles(ctx, diagnosticsConf, s3Prefix, heapBuf, goroutineBuf); err != nil {
		return errors.Wrap(err, "upload pprof data to S3")
	}

	const gb = 1 << 30
	grip.Notice(ctx, message.Fields{
		"message":          "pod diagnostics triggered",
		"trigger_reason":   triggerReason,
		"memory_usage_pct": fmt.Sprintf("%.1f%%", usagePercent*100),
		"memory_usage_gb":  fmt.Sprintf("%.2f", float64(usageBytes)/float64(gb)),
		"memory_limit_gb":  fmt.Sprintf("%.2f", float64(limitBytes)/float64(gb)),
		"hostname":         hostname,
		"s3_bucket":        diagnosticsConf,
		"s3_key_base":      s3Prefix,
		"job":              j.ID(),
	})

	return nil
}

// uploadPprofFiles uploads heap and goroutine profiles to S3.
func (j *podDiagnosticsJob) uploadPprofFiles(ctx context.Context, cfg evergreen.DiagnosticsConfig, s3Prefix string, heapBuf, goroutineBuf *bytes.Buffer) error {
	prefix := cfg.S3Prefix
	if prefix == "" {
		prefix = "pprof"
	}
	bucket, err := pail.NewS3Bucket(ctx, pail.S3Options{
		Name:   cfg.S3BucketName,
		Prefix: prefix,
	})
	if err != nil {
		return errors.Wrap(err, "creating S3 bucket client")
	}

	catcher := grip.NewBasicCatcher()
	if heapBuf != nil && heapBuf.Len() > 0 {
		catcher.Wrap(bucket.Put(ctx, s3Prefix+"-heap", heapBuf), "uploading heap profile")
	}
	if goroutineBuf != nil && goroutineBuf.Len() > 0 {
		catcher.Wrap(bucket.Put(ctx, s3Prefix+"-goroutine", goroutineBuf), "uploading goroutine profile")
	}
	return catcher.Resolve()
}

// readCgroupMemory returns the current memory usage and limit in bytes for the
// running container. It tries cgroups v2 first, then falls back to v1.
// kim: TODO: confirm whether this correctly returns memory usage.
// kim: TODO: confirm that this is a fast operation.
func readCgroupMemory() (usageBytes, limitBytes int64, err error) {
	startedAt := time.Now()
	defer func() {
		grip.Debug(context.Background(), message.Fields{
			"message":     "kim: read cgroup memory",
			"duration_ms": time.Since(startedAt).Milliseconds(),
		})
	}()

	// kim: TODO: determine if app servers are using cgroups v1 or v2. We may
	// only need to support one of them, which would simplify things.
	// cgroups v2
	if usage, err := readCgroupInt("/sys/fs/cgroup/memory.current"); err == nil {
		if limit, err := readCgroupInt("/sys/fs/cgroup/memory.max"); err == nil {
			return usage, limit, nil
		}
	}

	// cgroups v1 fallback
	usage, err := readCgroupInt("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading cgroup memory usage from cgroups v1 and v2")
	}
	limit, err := readCgroupInt("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading cgroup memory limit from cgroups v1 and v2")
	}
	return usage, limit, nil
}

// readCgroupInt reads a single integer from a cgroup file. Returns an error if
// the file is absent or contains "max" (meaning no limit is configured).
func readCgroupInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, errors.Wrapf(err, "reading file '%s'", path)
	}
	s := strings.TrimSpace(string(data))
	if s == "max" {
		// kim: TODO: double-check that callers handle this correctly
		return 0, errors.Errorf("file '%s' reports no limit (value: max)", path)
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parsing value '%s' from file '%s' as an integer", s, path)
	}
	return val, nil
}
