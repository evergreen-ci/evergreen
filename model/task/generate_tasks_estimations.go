package task

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	lookBackTime = 7 * 24 * time.Hour // one week
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

	results, err := getBatchedGenerateTasksEstimations(ctx, project, buildVariant, displayNames, lookBackTime)
	if err != nil {
		return nil, errors.Wrap(err, "getting generate tasks estimations")
	}

	for _, r := range results {
		result[r.DisplayName] = GenerateTasksEstimation{
			EstimatedNumGeneratedTasks:          int(math.Round(r.EstimatedCreated)),
			EstimatedNumActivatedGeneratedTasks: int(math.Round(r.EstimatedActivated)),
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
