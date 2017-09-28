package alerts

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
)

// Host Triggers

type SpawnTwoHourWarning struct{}

func (sthw SpawnTwoHourWarning) Id() string { return alertrecord.SpawnHostTwoHourWarning }

func (sthw SpawnTwoHourWarning) Display() string {
	return "Spawn host is due to expire in two hours"
}

func (sthw SpawnTwoHourWarning) CreateAlertRecord(ctx triggerContext) *alertrecord.AlertRecord {
	rec := newAlertRecord(ctx, alertrecord.SpawnHostTwoHourWarning)
	return rec
}

func (sthw SpawnTwoHourWarning) ShouldExecute(ctx triggerContext) (bool, error) {
	if ctx.host == nil || ctx.host.ExpirationTime.IsZero() || time.Until(ctx.host.ExpirationTime) > (2*time.Hour) {
		return false, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByHostAlertRecordType(ctx.host.Id, alertrecord.SpawnHostTwoHourWarning))
	if err != nil {
		return false, err
	}
	return rec == nil, nil
}

type SpawnTwelveHourWarning struct{}

func (sthw SpawnTwelveHourWarning) Id() string { return alertrecord.SpawnHostTwoHourWarning }

func (sthw SpawnTwelveHourWarning) Display() string {
	return "Spawn host is due to expire in twelve hours"
}

func (sthw SpawnTwelveHourWarning) CreateAlertRecord(ctx triggerContext) *alertrecord.AlertRecord {
	rec := newAlertRecord(ctx, alertrecord.SpawnHostTwelveHourWarning)
	return rec
}

func (sthw SpawnTwelveHourWarning) ShouldExecute(ctx triggerContext) (bool, error) {
	if ctx.host == nil || ctx.host.ExpirationTime.IsZero() || time.Until(ctx.host.ExpirationTime) > (12*time.Hour) {
		return false, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByHostAlertRecordType(ctx.host.Id, alertrecord.SpawnHostTwelveHourWarning))
	if err != nil {
		return false, err
	}
	return rec == nil, nil
}

type SpawnFailure struct{}

func (sf SpawnFailure) Id() string { return alertrecord.SpawnFailed }

func (sf SpawnFailure) Display() string {
	return "Spawn host startup failed."
}

func (sf SpawnFailure) CreateAlertRecord(ctx triggerContext) *alertrecord.AlertRecord {
	return nil
}

func (sf SpawnFailure) ShouldExecute(ctx triggerContext) (bool, error) {
	return true, nil
}

type SlowProvisionWarning struct{}

func (sthw SlowProvisionWarning) Id() string { return alertrecord.SlowProvisionWarning }

func (sthw SlowProvisionWarning) Display() string {
	return "Host is taking a long time to provision"
}

func (sthw SlowProvisionWarning) CreateAlertRecord(ctx triggerContext) *alertrecord.AlertRecord {
	rec := newAlertRecord(ctx, alertrecord.SlowProvisionWarning)
	return rec
}

func (sthw SlowProvisionWarning) ShouldExecute(ctx triggerContext) (bool, error) {
	// don't execute if the host is actually provisioned, or if it's been less than 20 minutes
	// since creation time
	if ctx.host.Provisioned || ctx.host.CreationTime.Before(time.Now().Add(-20*time.Minute)) {
		return false, nil
	}
	rec, err := alertrecord.FindOne(alertrecord.ByHostAlertRecordType(ctx.host.Id, alertrecord.SlowProvisionWarning))
	if err != nil {
		return false, err
	}
	return rec == nil, nil
}

type ProvisionFailed struct{}

func (pf ProvisionFailed) Id() string { return alertrecord.ProvisionFailed }

func (pf ProvisionFailed) Display() string {
	return "Host failed to provision"
}

func (pf ProvisionFailed) CreateAlertRecord(ctx triggerContext) *alertrecord.AlertRecord {
	// No bookkeeping done for this trigger - since it is triggered synchronously in only one place.
	return nil
}

func (pf *ProvisionFailed) ShouldExecute(ctx triggerContext) (bool, error) {
	// don't execute if the host is actually provisioned
	if ctx.host.Provisioned || ctx.host.Status == evergreen.HostRunning {
		return false, nil
	}
	return true, nil
}
