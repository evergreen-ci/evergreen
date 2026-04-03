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
	highMemoryDuration      = 60 * time.Second
	criticalMemoryThreshold = 0.90
	criticalMemoryDuration  = 5 * time.Second
)

// podDiagnosticsState tracks threshold crossing durations across job runs.
// kim: TODO: see if this can be not a global variable, or otherwise named
// better.
var (
	podDiagnosticsStateMu sync.Mutex
	above70PctSince       time.Time
	above90PctSince       time.Time
	lastCollectionAt      time.Time
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
	// kim: TODO: check if scopes works in a local queue. It may also be
	// sufficient to just have this run once per minute, but need to confirm
	// that the memory check is sufficiently fast.
	j.SetScopes([]string{podDiagnosticsJobName})
	j.SetEnqueueAllScopes(true)
	return j
}

func (j *podDiagnosticsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	usageBytes, limitBytes, err := readCgroupMemory()
	if err != nil {
		// Cgroup files won't exist outside container environments; warn but
		// don't surface this as a job error.
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "reading cgroup memory stats for pod diagnostics",
			"job":     j.ID(),
		}))
		return
	}
	if limitBytes == 0 {
		return // container has no memory limit configured
	}

	usagePct := float64(usageBytes) / float64(limitBytes)

	podDiagnosticsStateMu.Lock()
	defer podDiagnosticsStateMu.Unlock()

	now := time.Now()

	// Update duration-tracking state based on current usage.
	if usagePct >= criticalMemoryThreshold {
		if above90PctSince.IsZero() {
			above90PctSince = now
		}
		if above70PctSince.IsZero() {
			above70PctSince = now
		}
	} else if usagePct >= highMemoryThreshold {
		above90PctSince = time.Time{} // dropped below critical, reset that timer
		if above70PctSince.IsZero() {
			above70PctSince = now
		}
	} else {
		above70PctSince = time.Time{}
		above90PctSince = time.Time{}
		return // memory is healthy
	}

	// Enforce cooldown to avoid uploading profiles on every job tick during a
	// prolonged spike.
	if !lastCollectionAt.IsZero() {
		return
	}

	// Check whether either threshold condition has been met.
	var triggerReason string
	switch {
	case !above90PctSince.IsZero() && now.Sub(above90PctSince) >= criticalMemoryDuration:
		triggerReason = fmt.Sprintf(
			"memory at %.1f%% of container limit for %.0fs (threshold: >90%% for >5s)",
			usagePct*100, now.Sub(above90PctSince).Seconds())
	case !above70PctSince.IsZero() && now.Sub(above70PctSince) >= highMemoryDuration:
		triggerReason = fmt.Sprintf(
			"memory at %.1f%% of container limit for %.0fs (threshold: >70%% for >60s)",
			usagePct*100, now.Sub(above70PctSince).Seconds())
	default:
		return // neither threshold condition met yet
	}

	// Reset timers and record collection time before releasing the lock so
	// the next job invocation doesn't re-trigger during the cooldown window.
	above70PctSince = time.Time{}
	above90PctSince = time.Time{}
	lastCollectionAt = now

	// Collect and upload outside the lock (state is already reset above).
	j.collectAndUpload(ctx, usageBytes, limitBytes, usagePct, triggerReason)
}

func (j *podDiagnosticsJob) collectAndUpload(ctx context.Context, usageBytes, limitBytes int64, usagePct float64, triggerReason string, ts time.Time) error {
	settings := j.env.Settings()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	s3Prefix := fmt.Sprintf("%s/%s", hostname, time.Now().Format(TSFormat))

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

	cfg := settings.Diagnostics
	s3Uploaded := false
	if cfg.S3BucketName != "" {
		if uploadErr := uploadPprofProfiles(ctx, cfg, s3Prefix, heapBuf, goroutineBuf); uploadErr != nil {
			grip.Warning(ctx, message.WrapError(uploadErr, message.Fields{
				"message": "could not upload pprof profiles to S3",
				"job":     j.ID(),
				"bucket":  cfg.S3BucketName,
			}))
		} else {
			s3Uploaded = true
		}
	}

	grip.Notice(ctx, message.Fields{
		"message":          "pod diagnostics triggered",
		"trigger_reason":   triggerReason,
		"memory_usage_pct": fmt.Sprintf("%.1f%%", usagePct*100),
		"memory_usage_gb":  fmt.Sprintf("%.2f", float64(usageBytes)/(1<<30)),
		"memory_limit_gb":  fmt.Sprintf("%.2f", float64(limitBytes)/(1<<30)),
		"hostname":         hostname,
		"s3_bucket":        cfg.S3BucketName,
		"s3_key_base":      s3Prefix,
		"s3_uploaded":      s3Uploaded,
		"job":              j.ID(),
	})

	return nil
}

// uploadPprofProfiles uploads heap and goroutine profiles to S3. nil buffers
// are skipped.
func uploadPprofProfiles(ctx context.Context, cfg evergreen.DiagnosticsConfig, s3Prefix string, heapBuf, goroutineBuf *bytes.Buffer) error {
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
func readCgroupMemory() (usageBytes, limitBytes int64, err error) {
	// cgroups v2
	if usage, err := readCgroupInt("/sys/fs/cgroup/memory.current"); err == nil {
		if limit, err := readCgroupInt("/sys/fs/cgroup/memory.max"); err == nil {
			return usage, limit, nil
		}
	}
	// cgroups v1 fallback
	usage, err := readCgroupInt("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading cgroup memory usage (tried cgroups v2 and v1 paths)")
	}
	limit, err := readCgroupInt("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading cgroup memory limit")
	}
	return usage, limit, nil
}

// readCgroupInt reads a single integer from a cgroup file. Returns an error if
// the file is absent or contains "max" (meaning no limit is configured).
func readCgroupInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, errors.Wrapf(err, "reading '%s'", path)
	}
	s := strings.TrimSpace(string(data))
	if s == "max" {
		return 0, errors.Errorf("'%s' reports no limit (value: max)", path)
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parsing value '%s' from '%s'", s, path)
	}
	return val, nil
}
