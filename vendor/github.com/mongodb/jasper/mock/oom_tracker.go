package mock

import "context"

// OOMTracker provides a mock implementation of OOM detection for
// testing.
type OOMTracker struct {
	PIDs         []int
	WasOOMKilled bool
	FailCheck    bool
	FailClear    bool
}

// Check returns an error if FailCheck is set, nil otherwise.
func (o *OOMTracker) Check(context.Context) error {
	if o.FailCheck {
		return mockFail()
	}

	return nil
}

// Clear returns an error if FailCheck is set, nil otherwise.
func (o *OOMTracker) Clear(context.Context) error {
	if o.FailClear {
		return mockFail()
	}

	return nil
}

// Report returns the value of the WasOOMKilled and PIDs fields.
func (o *OOMTracker) Report() (bool, []int) {
	return o.WasOOMKilled, o.PIDs
}
