package internal

import (
	"time"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/poplar"
	"github.com/golang/protobuf/ptypes"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pkg/errors"
)

func ExportArtifactInfo(in *poplar.TestArtifact) *gopb.ArtifactInfo {
	out := &gopb.ArtifactInfo{
		Bucket: in.Bucket,
		Prefix: in.Prefix,
		Path:   in.Path,
		Tags:   in.Tags,
	}

	ts, err := ExportTimestamp(in.CreatedAt)
	if err == nil {
		out.CreatedAt = ts
	}

	switch {
	case in.PayloadTEXT:
		out.Format = gopb.DataFormat_TEXT
	case in.PayloadFTDC:
		out.Format = gopb.DataFormat_FTDC
	case in.PayloadBSON:
		out.Format = gopb.DataFormat_BSON
	case in.PayloadJSON:
		out.Format = gopb.DataFormat_JSON
	case in.PayloadCSV:
		out.Format = gopb.DataFormat_CSV
	}

	switch {
	case in.DataUncompressed:
		out.Compression = gopb.CompressionType_NONE
	case in.DataGzipped:
		out.Compression = gopb.CompressionType_GZ
	case in.DataTarball:
		out.Compression = gopb.CompressionType_TARGZ
	}

	switch {
	case in.EventsRaw:
		out.Schema = gopb.SchemaType_RAW_EVENTS
	case in.EventsHistogram:
		out.Schema = gopb.SchemaType_HISTOGRAM
	case in.EventsIntervalSummary:
		out.Schema = gopb.SchemaType_INTERVAL_SUMMARIZATION
	case in.EventsCollapsed:
		out.Schema = gopb.SchemaType_COLLAPSED_EVENTS
	}

	return out
}

func ExportRollup(in *poplar.TestMetrics) *gopb.RollupValue {
	out := &gopb.RollupValue{
		Name:          in.Name,
		Version:       int64(in.Version),
		UserSubmitted: true,
	}

	if in.Type != "" {
		t, ok := gopb.RollupType_value[in.Type]
		if ok {
			out.Type = gopb.RollupType(t)
		}
	}

	switch val := in.Value.(type) {
	case int64:
		out.Value = &gopb.RollupValue_Int{Int: val}
	case float64:
		out.Value = &gopb.RollupValue_Fl{Fl: val}
	case int:
		out.Value = &gopb.RollupValue_Int{Int: int64(val)}
	case int32:
		out.Value = &gopb.RollupValue_Int{Int: int64(val)}
	case float32:
		out.Value = &gopb.RollupValue_Fl{Fl: float64(val)}
	}

	return out
}

func ExportTimestamp(t time.Time) (*timestamp.Timestamp, error) {
	var ts *timestamp.Timestamp
	var err error
	if !t.IsZero() {
		ts, err = ptypes.TimestampProto(t)
		if err != nil {
			return nil, errors.Wrap(err, "problem specifying timestamp")
		}
	}

	return ts, nil
}
