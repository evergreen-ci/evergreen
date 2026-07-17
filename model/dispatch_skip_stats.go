package model

import "context"

type dispatchSkipStatsKey struct{}

// DispatchSkipStats tracks the reasons tasks were skipped during a single
// next_task dispatch attempt.
type DispatchSkipStats struct {
	DependenciesNotMetCached   int `json:"dependencies_not_met_cached"`
	AlreadyDispatched          int `json:"already_dispatched"`
	TaskAlreadyStarted         int `json:"task_already_started"`
	GenerateTaskLimitExceeded  int `json:"generate_task_limit_exceeded"`
	LargeParserProjectExceeded int `json:"large_parser_project_exceeded"`
	DependenciesNotMetDB       int `json:"dependencies_not_met_db"`
	AMIOutdated                int `json:"ami_outdated"`
	TaskGroupMaxHosts          int `json:"task_group_max_hosts"`
	TaskGroupNoDispatchable    int `json:"task_group_no_dispatchable"`
	BlockedSingleHostTaskGroup int `json:"blocked_single_host_task_group"`
	NotHostDispatchable        int `json:"not_host_dispatchable"`
	ProjectDispatchDisabled    int `json:"project_dispatch_disabled"`
	TaskBlocked                int `json:"task_blocked"`
	SingleTaskDistroMismatch   int `json:"single_task_distro_mismatch"`
	DispatchRace               int `json:"dispatch_race"`
}

// HasOnlyExpectedSkips returns true if at least one task was skipped for a
// healthy reason that the queue's dependency-met count does not account for
// (task group constraints, AMI mismatches, etc.) and no task was skipped for a reason
// indicating the queue contains tasks that should not be dispatchable at all.
func (s *DispatchSkipStats) HasOnlyExpectedSkips() bool {
	if s == nil {
		return false
	}

	unexpectedSkips := s.NotHostDispatchable +
		s.ProjectDispatchDisabled +
		s.TaskBlocked

	expectedSkips := s.AlreadyDispatched +
		s.TaskAlreadyStarted +
		s.GenerateTaskLimitExceeded +
		s.LargeParserProjectExceeded +
		s.DependenciesNotMetDB +
		s.AMIOutdated +
		s.TaskGroupMaxHosts +
		s.TaskGroupNoDispatchable +
		s.BlockedSingleHostTaskGroup +
		s.SingleTaskDistroMismatch +
		s.DispatchRace

	return expectedSkips > 0 && unexpectedSkips == 0
}

// NewDispatchSkipStatsContext returns a context with a fresh DispatchSkipStats attached.
func NewDispatchSkipStatsContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, dispatchSkipStatsKey{}, &DispatchSkipStats{})
}

// DispatchSkipStatsFromContext retrieves the DispatchSkipStats from the context.
func DispatchSkipStatsFromContext(ctx context.Context) *DispatchSkipStats {
	if stats, ok := ctx.Value(dispatchSkipStatsKey{}).(*DispatchSkipStats); ok {
		return stats
	}
	return &DispatchSkipStats{}
}
