package internal

import "github.com/evergreen-ci/poplar"

func (rt CreateOptions_RecorderType) Export() poplar.RecorderType {
	switch rt {
	case CreateOptions_PERF:
		return poplar.RecorderPerf
	case CreateOptions_PERF_SINGLE:
		return poplar.RecorderPerfSingle
	case CreateOptions_PERF_100MS:
		return poplar.RecorderPerf100ms
	case CreateOptions_PERF_1S:
		return poplar.RecorderPerf1s
	case CreateOptions_HISTOGRAM_SINGLE:
		return poplar.RecorderHistogramSingle
	case CreateOptions_HISTOGRAM_100MS:
		return poplar.RecorderHistogram100ms
	case CreateOptions_HISTOGRAM_1S:
		return poplar.RecorderHistogram1s
	case CreateOptions_INTERVAL_SUMMARIZATION:
		return poplar.CustomMetrics
	default:
		return ""
	}
}

func (ect CreateOptions_EventsCollectorType) Export() poplar.EventsCollectorType { //nolint: misspell
	switch ect { //nolint: misspell
	case CreateOptions_BASIC:
		return poplar.EventsCollectorBasic
	case CreateOptions_PASSTHROUGH:
		return poplar.EventsCollectorPassthrough
	case CreateOptions_SAMPLING_100:
		return poplar.EventsCollectorSampling100
	case CreateOptions_SAMPLING_1K:
		return poplar.EventsCollectorSampling1k
	case CreateOptions_SAMPLING_10K:
		return poplar.EventsCollectorSampling10k
	case CreateOptions_SAMPLING_100K:
		return poplar.EventsCollectorSampling100k
	case CreateOptions_RAND_SAMPLING_50:
		return poplar.EventsCollectorRandomSampling50
	case CreateOptions_RAND_SAMPLING_25:
		return poplar.EventsCollectorRandomSampling25
	case CreateOptions_RAND_SAMPLING_10:
		return poplar.EventsCollectorRandomSampling10
	case CreateOptions_INTERVAL_100MS:
		return poplar.EventsCollectorInterval100ms
	case CreateOptions_INTERVAL_1S:
		return poplar.EventsCollectorInterval1s
	default:
		return ""
	}

}

func (opts *CreateOptions) Export() poplar.CreateOptions {
	return poplar.CreateOptions{
		Path:      opts.Path,
		ChunkSize: int(opts.ChunkSize),
		Streaming: opts.Streaming,
		Dynamic:   opts.Dynamic,
		Buffered:  opts.Buffered,
		Recorder:  opts.Recorder.Export(),
		Events:    opts.Events.Export(),
	}
}
