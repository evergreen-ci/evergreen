package task

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	lookBackTime = 7 * 24 * time.Hour // one week
)

// generateTasksEstimationCache shares estimations across builds that create the
// same generator tasks, which otherwise re-ran this aggregate on every build
// creation. Like the expected-duration cache it's keyed by the (project, build
// variant, task name) tuple the estimate is derived from.
var generateTasksEstimationCache = expirable.NewLRU[string, GenerateTasksEstimation](estimateCacheMaxSize, nil, predictionTTL)

// GenerateTasksEstimation holds estimation results for a single generator task.
type GenerateTasksEstimation struct {
	EstimatedNumGeneratedTasks          int
	EstimatedNumActivatedGeneratedTasks int
}

// GetBatchedGenerateTasksEstimations returns a map of estimations for multiple generator tasks, where keys
// are each task's display name.
func GetBatchedGenerateTasksEstimations(ctx context.Context, project, buildVariant string, displayNames []string) (map[string]GenerateTasksEstimation, error) {
	result := make(map[string]GenerateTasksEstimation, len(displayNames))
	if len(displayNames) == 0 {
		return result, nil
	}

	ctx, span := tracer.Start(ctx, "get-generate-tasks-estimations", trace.WithAttributes(
		attribute.String(evergreen.ProjectIdentifierOtelAttribute, project),
		attribute.String(evergreen.BuildNameOtelAttribute, buildVariant),
		attribute.Int("evergreen.task.num_generators", len(displayNames)),
	))
	defer span.End()

	uncached := make([]string, 0, len(displayNames))
	for _, name := range displayNames {
		if est, ok := generateTasksEstimationCache.Get(estimateCacheKey(project, buildVariant, name)); ok {
			result[name] = est
			continue
		}
		uncached = append(uncached, name)
	}
	if len(uncached) == 0 {
		return result, nil
	}

	results, err := getBatchedGenerateTasksEstimations(ctx, project, buildVariant, uncached, lookBackTime)
	if err != nil {
		return nil, errors.Wrap(err, "getting generate tasks estimations")
	}

	for _, r := range results {
		est := GenerateTasksEstimation{
			EstimatedNumGeneratedTasks:          int(math.Round(r.EstimatedCreated)),
			EstimatedNumActivatedGeneratedTasks: int(math.Round(r.EstimatedActivated)),
		}
		result[r.DisplayName] = est
		generateTasksEstimationCache.Add(estimateCacheKey(project, buildVariant, r.DisplayName), est)
	}

	return result, nil
}

// SetGenerateTasksEstimationsFromMap applies generate.tasks estimation results to a task.
func (t *Task) SetGenerateTasksEstimationsFromMap(estimations map[string]GenerateTasksEstimation) {
	if !t.GenerateTask {
		return
	}
	est, ok := estimations[t.DisplayName]
	if !ok {
		t.EstimatedNumGeneratedTasks = utility.ToIntPtr(0)
		t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(0)
		return
	}

	t.EstimatedNumGeneratedTasks = utility.ToIntPtr(est.EstimatedNumGeneratedTasks)
	t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(est.EstimatedNumActivatedGeneratedTasks)
}
