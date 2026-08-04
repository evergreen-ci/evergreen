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

var (
	generateTasksEstimationCache = expirable.NewLRU[estimateCacheKey, GenerateTasksEstimation](estimateCacheMaxSize, nil, estimateCacheTTL)

	// Generators with no history get their own cache so they expire sooner than a real estimate. Without it, one
	// generator missing from the results puts the whole batch back on the uncached path.
	noGenerateTasksHistoryCache = expirable.NewLRU[estimateCacheKey, struct{}](estimateCacheMaxSize, nil, noHistoryCacheTTL)
)

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

	numHits, numNegativeHits := 0, 0
	uncached := make([]string, 0, len(displayNames))
	for _, name := range displayNames {
		key := estimateCacheKey{project: project, buildVariant: buildVariant, taskDisplayName: name}
		if est, ok := generateTasksEstimationCache.Get(key); ok {
			result[name] = est
			numHits++
			continue
		}
		if _, ok := noGenerateTasksHistoryCache.Get(key); ok {
			numNegativeHits++
			continue
		}
		uncached = append(uncached, name)
	}
	span.SetAttributes(
		attribute.Int("evergreen.task.num_cache_hits", numHits),
		attribute.Int("evergreen.task.num_negative_cache_hits", numNegativeHits),
		attribute.Int("evergreen.task.num_cache_misses", len(uncached)),
		attribute.Int("evergreen.task.generate_tasks_estimation_cache_size", generateTasksEstimationCache.Len()),
		attribute.Int("evergreen.task.no_generate_tasks_history_cache_size", noGenerateTasksHistoryCache.Len()),
	)
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
		generateTasksEstimationCache.Add(estimateCacheKey{project: project, buildVariant: buildVariant, taskDisplayName: r.DisplayName}, est)
	}

	// Absence from the results means no history. Caching it as an estimate would be indistinguishable from a real zero.
	for _, name := range uncached {
		if _, ok := result[name]; !ok {
			noGenerateTasksHistoryCache.Add(estimateCacheKey{project: project, buildVariant: buildVariant, taskDisplayName: name}, struct{}{})
		}
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
