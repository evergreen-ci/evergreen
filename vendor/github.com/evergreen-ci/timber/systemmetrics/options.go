package systemmetrics

import "github.com/pkg/errors"

// SystemMetricsOptions support the use and creation of a system metrics object.
type SystemMetricsOptions struct {
	// Unique information to identify the system metrics object.
	Project   string
	Version   string
	Variant   string
	TaskName  string
	TaskId    string
	Execution int32
	Mainline  bool

	// Data storage information for this object
	Compression CompressionType
	Schema      SchemaType
}

// MetricDataOptions describes the data being streamed or sent to cedar.
type MetricDataOptions struct {
	Id         string
	MetricType string
	Format     DataFormat
}

func (s *MetricDataOptions) validate() error {
	if s.Id == "" {
		return errors.New("must specify id of system metrics object")
	}
	if s.MetricType == "" {
		return errors.New("must specify the the type of metric in data")
	}
	if err := s.Format.validate(); err != nil {
		return errors.Wrapf(err, "invalid format for id %s", s.Id)
	}
	return nil
}
