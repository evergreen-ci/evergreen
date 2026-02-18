package patch

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var packageName = fmt.Sprintf("%s%s", evergreen.PackageName, "/model/patch")

var tracer = otel.GetTracerProvider().Tracer(packageName)

// OpenTelemetry span names for merge queue lifecycle events.
const (
	MergeQueueIntentCreatedSpan    = "merge_queue.intent_created"
	MergeQueuePatchProcessingSpan  = "merge_queue.patch_processing"
	MergeQueuePatchCompletedSpan   = "merge_queue.patch_completed"
)

// OpenTelemetry attribute keys for merge queue metrics.
const (
	MergeQueueAttrOrg                   = "evergreen.merge_queue.org"
	MergeQueueAttrRepo                  = "evergreen.merge_queue.repo"
	MergeQueueAttrQueueName             = "evergreen.merge_queue.queue_name"
	MergeQueueAttrBaseBranch            = "evergreen.merge_queue.base_branch"
	MergeQueueAttrHeadSHA               = "evergreen.merge_queue.head_sha"
	MergeQueueAttrPatchID               = "evergreen.merge_queue.patch_id"
	MergeQueueAttrProjectID             = "evergreen.merge_queue.project_id"
	MergeQueueAttrGithubPRURL           = "evergreen.merge_queue.github_pr_url"
	MergeQueueAttrMsgID                 = "evergreen.merge_queue.msg_id"
	MergeQueueAttrTimeInQueueMs         = "evergreen.merge_queue.time_in_queue_ms"
	MergeQueueAttrTimeToFirstTaskMs     = "evergreen.merge_queue.time_to_first_task_ms"
	MergeQueueAttrStatus                = "evergreen.merge_queue.status"
	MergeQueueAttrRemovalReason         = "evergreen.merge_queue.removal_reason"
	MergeQueueAttrHasTestFailure        = "evergreen.merge_queue.has_test_failure"
	MergeQueueAttrHasSystemFailure      = "evergreen.merge_queue.has_system_failure"
	MergeQueueAttrHasSetupFailure       = "evergreen.merge_queue.has_setup_failure"
	MergeQueueAttrHasTimeoutFailure     = "evergreen.merge_queue.has_timeout_failure"
	MergeQueueAttrFailedTaskCount       = "evergreen.merge_queue.failed_task_count"
	MergeQueueAttrTotalTaskCount        = "evergreen.merge_queue.total_task_count"
	MergeQueueAttrHasRunningTasks       = "evergreen.merge_queue.has_running_tasks"
	MergeQueueAttrVariants              = "evergreen.merge_queue.variants"
	MergeQueueAttrSlowestTaskID         = "evergreen.merge_queue.slowest_task_id"
	MergeQueueAttrSlowestTaskName       = "evergreen.merge_queue.slowest_task_name"
	MergeQueueAttrSlowestTaskDurationMs = "evergreen.merge_queue.slowest_task_duration_ms"
	MergeQueueAttrSlowestTaskVariant    = "evergreen.merge_queue.slowest_variant"
)

// BuildMergeQueueSpanAttributes creates a slice of common trace attributes for merge queue operations.
func BuildMergeQueueSpanAttributes(org, repo, baseBranch, headSHA, githubPRURL string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(MergeQueueAttrOrg, org),
		attribute.String(MergeQueueAttrRepo, repo),
		attribute.String(MergeQueueAttrQueueName, baseBranch),
		attribute.String(MergeQueueAttrBaseBranch, baseBranch),
		attribute.String(MergeQueueAttrHeadSHA, headSHA),
		attribute.String(MergeQueueAttrGithubPRURL, githubPRURL),
	}
}
