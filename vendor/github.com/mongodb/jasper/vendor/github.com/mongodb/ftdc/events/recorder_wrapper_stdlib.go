package events

import "time"

// TimerManager is a subset of the testing.B tool, used to manage
// setup code.
type TimerManager interface {
	ResetTimer()
	StartTimer()
	StopTimer()
}

// NewShimRecorder takes a recorder and acts as a thin recorder, using
// the TimeManager interface for relevant Begin/Record/Reset
// values.
//
// Go's standard library testing package has a *B type for
// benchmarking that you can pass as a TimerManager.
func NewShimRecorder(r Recorder, tm TimerManager) Recorder {
	return &stdShim{
		b:        tm,
		Recorder: r,
	}
}

type stdShim struct {
	b TimerManager
	Recorder
}

func (r *stdShim) Reset() {
	r.b.ResetTimer()
	r.Recorder.Reset()
}
func (r *stdShim) Begin() {
	r.b.StartTimer()
	r.Recorder.Begin()
}
func (r *stdShim) End(dur time.Duration) {
	r.b.StopTimer()
	r.Recorder.End(dur)
}
