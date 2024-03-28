package graphql

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
)

// WholeWeekdaysOff is the resolver for the wholeWeekdaysOff field.
func (r *sleepScheduleInputResolver) WholeWeekdaysOff(ctx context.Context, obj *host.SleepScheduleInfo, data []int) error {
	weekdays := []time.Weekday{}
	for _, day := range data {
		weekdays = append(weekdays, time.Weekday(day))
	}
	obj.WholeWeekdaysOff = weekdays
	return nil
}

// SleepScheduleInput returns SleepScheduleInputResolver implementation.
func (r *Resolver) SleepScheduleInput() SleepScheduleInputResolver {
	return &sleepScheduleInputResolver{r}
}

type sleepScheduleInputResolver struct{ *Resolver }
