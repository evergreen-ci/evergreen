package poplar

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Report is the top level object to represent a suite of performance
// tests and is used to feed data to a ceder instance. All of the test
// data is in the "Tests" field, with additional metadata common to
// all tests in the top-level fields of the Report structure.
type Report struct {
	// These settings are at the top level to provide a DRY
	// location for the data, in the DB they're part of the
	// test-info, but we're going to assume that these tasks run
	// in Evergreen conventionally.
	Project   string `bson:"project" json:"project" yaml:"project"`
	Version   string `bson:"version" json:"version" yaml:"version"`
	Order     int    `bson:"order" json:"order" yaml:"order"`
	Variant   string `bson:"variant" json:"variant" yaml:"variant"`
	TaskName  string `bson:"task_name" json:"task_name" yaml:"task_name"`
	TaskID    string `bson:"task_id" json:"task_id" yaml:"task_id"`
	Execution int    `bson:"execution_number" json:"execution_number" yaml:"execution_number"`
	Mainline  bool   `bson:"mainline" json:"mainline" yaml:"mainline"`

	BucketConf BucketConfiguration `bson:"bucket" json:"bucket" yaml:"bucket"`

	// Tests holds all of the test data.
	Tests []Test `bson:"tests" json:"tests" yaml:"tests"`
}

// Test holds data about a specific test and its subtests. You should
// not populate the ID field, and instead populate the entire Info
// structure. ID fields are populated by the server by hashing the
// Info document along with high level metadata that is, in this
// representation, stored in the report structure.
type Test struct {
	ID          string         `bson:"_id" json:"id" yaml:"id"`
	Info        TestInfo       `bson:"info" json:"info" yaml:"info"`
	CreatedAt   time.Time      `bson:"created_at" json:"created_at" yaml:"created_at"`
	CompletedAt time.Time      `bson:"completed_at" json:"completed_at" yaml:"completed_at"`
	Artifacts   []TestArtifact `bson:"artifacts" json:"artifacts" yaml:"artifacts"`
	Metrics     []TestMetrics  `bson:"metrics" json:"metrics" yaml:"metrics"`
	SubTests    []Test         `bson:"sub_tests" json:"sub_tests" yaml:"sub_tests"`
}

// TestInfo holds metadata about the test configuration and
// execution. The parent field holds the content of the ID field of
// the parent test for sub tests, and should be populated
// automatically by the client when uploading results.
type TestInfo struct {
	TestName  string           `bson:"test_name" json:"test_name" yaml:"test_name"`
	Trial     int              `bson:"trial" json:"trial" yaml:"trial"`
	Parent    string           `bson:"parent" json:"parent" yaml:"parent"`
	Tags      []string         `bson:"tags" json:"tags" yaml:"tags"`
	Arguments map[string]int32 `bson:"args" json:"args" yaml:"args"`
}

// TestArtifact is an optional structure to allow you to upload and
// attach metadata to results files.
type TestArtifact struct {
	Bucket                string    `bson:"bucket" json:"bucket" yaml:"bucket"`
	Prefix                string    `bson:"prefix" json:"prefix" yaml:"prefix"`
	Permissions           string    `bson:"permissions" json:"permissions" yaml:"permissions"`
	Path                  string    `bson:"path" json:"path" yaml:"path"`
	Tags                  []string  `bson:"tags" json:"tags" yaml:"tags"`
	CreatedAt             time.Time `bson:"created_at" json:"created_at" yaml:"created_at"`
	LocalFile             string    `bson:"local_path,omitempty" json:"local_path,omitempty" yaml:"local_path,omitempty"`
	PayloadTEXT           bool      `bson:"is_text,omitempty" json:"is_text,omitempty" yaml:"is_text,omitempty"`
	PayloadFTDC           bool      `bson:"is_ftdc,omitempty" json:"is_ftdc,omitempty" yaml:"is_ftdc,omitempty"`
	PayloadBSON           bool      `bson:"is_bson,omitempty" json:"is_bson,omitempty" yaml:"is_bson,omitempty"`
	PayloadJSON           bool      `bson:"is_json,omitempty" json:"is_json,omitempty" yaml:"is_json,omitempty"`
	PayloadCSV            bool      `bson:"is_csv,omitempty" json:"is_csv,omitempty" yaml:"is_csv,omitempty"`
	DataUncompressed      bool      `bson:"is_uncompressed" json:"is_uncompressed" yaml:"is_uncompressed"`
	DataGzipped           bool      `bson:"is_gzip,omitempty" json:"is_gzip,omitempty" yaml:"is_gzip,omitempty"`
	DataTarball           bool      `bson:"is_tarball,omitempty" json:"is_tarball,omitempty" yaml:"is_tarball,omitempty"`
	EventsRaw             bool      `bson:"events_raw,omitempty" json:"events_raw,omitempty" yaml:"events_raw,omitempty"`
	EventsHistogram       bool      `bson:"events_histogram,omitempty" json:"events_histogram,omitempty" yaml:"events_histogram,omitempty"`
	EventsIntervalSummary bool      `bson:"events_interval_summary,omitempty" json:"events_interval_summary,omitempty" yaml:"events_interval_summary,omitempty"`
	EventsCollapsed       bool      `bson:"events_collapsed,omitempty" json:"events_collapsed,omitempty" yaml:"events_collapsed,omitempty"`
	ConvertGzip           bool      `bson:"convert_gzip,omitempty" json:"convert_gzip,omitempty" yaml:"convert_gzip,omitempty"`
	ConvertBSON2FTDC      bool      `bson:"convert_bson_to_ftdc,omitempty" json:"convert_bson_to_ftdc,omitempty" yaml:"convert_bson_to_ftdc,omitempty"`
	ConvertJSON2FTDC      bool      `bson:"convert_json_to_ftdc" json:"convert_json_to_ftdc" yaml:"convert_json_to_ftdc"`
	ConvertCSV2FTDC       bool      `bson:"convert_csv_to_ftdc" json:"convert_csv_to_ftdc" yaml:"convert_csv_to_ftdc"`
}

// Validate examines an entire artifact structure and reports if there
// are any logical inconsistencies with the data.
func (a *TestArtifact) Validate() error {
	catcher := grip.NewBasicCatcher()

	if a.ConvertGzip {
		a.DataGzipped = true
	}

	if a.ConvertCSV2FTDC {
		a.PayloadCSV = false
		a.PayloadFTDC = true

	}

	if a.ConvertBSON2FTDC {
		a.PayloadBSON = false
		a.PayloadFTDC = true
	}

	if a.ConvertJSON2FTDC {
		a.PayloadJSON = false
		a.PayloadFTDC = true
	}

	if isMoreThanOneTrue([]bool{a.ConvertBSON2FTDC, a.ConvertCSV2FTDC, a.ConvertJSON2FTDC, a.ConvertGzip}) {
		catcher.Add(errors.New("cannot specify contradictory conversion requests"))
	}

	if isMoreThanOneTrue([]bool{a.PayloadBSON, a.PayloadJSON, a.PayloadCSV, a.PayloadTEXT, a.PayloadFTDC}) {
		catcher.Add(errors.New("must specify exactly one payload type"))
	}

	if isMoreThanOneTrue([]bool{a.DataGzipped, a.DataTarball, a.DataUncompressed}) {
		catcher.Add(errors.New("must specify exactly one file format type"))
	}

	if isMoreThanOneTrue([]bool{a.EventsCollapsed, a.EventsHistogram, a.EventsIntervalSummary, a.EventsRaw}) {
		catcher.Add(errors.New("must specify exactly one event format type"))
	}

	return catcher.Resolve()
}

// TestMetrics is a structure that holds computed metrics for an
// entire test in the case that test harnesses need or want to report
// their own test outcomes.
type TestMetrics struct {
	Name    string      `bson:"name" json:"name" yaml:"name"`
	Version int         `bson:"version,omitempty" json:"version,omitempty" yaml:"version,omitempty"`
	Type    string      `bson:"type" json:"type" yaml:"type"`
	Value   interface{} `bson:"value" json:"value" yaml:"value"`
}

// BucketConfiguration describes the configuration information for an
// AWS S3 bucket for uploading test artifacts for this report.
type BucketConfiguration struct {
	APIKey    string `bson:"api_key" json:"api_key" yaml:"api_key"`
	APISecret string `bson:"api_secret" json:"api_secret" yaml:"api_secret"`
	APIToken  string `bson:"api_token" json:"api_token" yaml:"api_token"`
	Name      string `bson:"name" json:"name" yaml:"name"`
	Prefix    string `bson:"prefix" json:"prefix" yaml:"prefix"`
	Region    string `bson:"region" json:"region" yaml:"region"`
}
