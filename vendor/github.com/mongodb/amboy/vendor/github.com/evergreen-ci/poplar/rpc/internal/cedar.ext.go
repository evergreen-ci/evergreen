package internal

import (
	"time"

	"github.com/evergreen-ci/poplar"
	"github.com/golang/protobuf/ptypes"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pkg/errors"
)

func ExportArtifactInfo(in *poplar.TestArtifact) *ArtifactInfo {
	out := &ArtifactInfo{
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
		out.Format = DataFormat_TEXT
	case in.PayloadFTDC:
		out.Format = DataFormat_FTDC
	case in.PayloadBSON:
		out.Format = DataFormat_BSON
	case in.PayloadJSON:
		out.Format = DataFormat_JSON
	case in.PayloadCSV:
		out.Format = DataFormat_CSV
	}

	switch {
	case in.DataUncompressed:
		out.Compression = CompressionType_NONE
	case in.DataGzipped:
		out.Compression = CompressionType_GZ
	case in.DataTarball:
		out.Compression = CompressionType_TARGZ
	}

	switch {
	case in.EventsRaw:
		out.Schema = SchemaType_RAW_EVENTS
	case in.EventsHistogram:
		out.Schema = SchemaType_HISTOGRAM
	case in.EventsIntervalSummary:
		out.Schema = SchemaType_INTERVAL_SUMMARIZATION
	case in.EventsCollapsed:
		out.Schema = SchemaType_COLLAPSED_EVENTS
	}

	return out
}

func ExportRollup(in *poplar.TestMetrics) *RollupValue {
	out := &RollupValue{
		Name:          in.Name,
		Version:       int64(in.Version),
		UserSubmitted: true,
	}

	if in.Type != "" {
		t, ok := RollupType_value[in.Type]
		if ok {
			out.Type = RollupType(t)
		}
	}

	switch val := in.Value.(type) {
	case int64:
		out.Value = &RollupValue_Int{Int: val}
	case float64:
		out.Value = &RollupValue_Fl{Fl: val}
	case int:
		out.Value = &RollupValue_Int{Int: int64(val)}
	case int32:
		out.Value = &RollupValue_Int{Int: int64(val)}
	case float32:
		out.Value = &RollupValue_Fl{Fl: float64(val)}
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
