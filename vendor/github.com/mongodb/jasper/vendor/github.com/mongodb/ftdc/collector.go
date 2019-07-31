package ftdc

// Collector describes the interface for collecting and constructing
// FTDC data series. Implementations may have different efficiencies
// and handling of schema changes.
//
// The SetMetadata and Add methods both take interface{} values. These
// are converted to bson documents; however it is an error to pass a
// type based on a map.
type Collector interface {
	// SetMetadata sets the metadata document for the collector or
	// chunk. This document is optional. Pass a nil to unset it,
	// or a different document to override a previous operation.
	SetMetadata(interface{}) error

	// Add extracts metrics from a document and appends it to the
	// current collector. These documents MUST all be
	// identical including field order. Returns an error if there
	// is a problem parsing the document or if the number of
	// metrics collected changes.
	Add(interface{}) error

	// Resolve renders the existing documents and outputs the full
	// FTDC chunk as a byte slice to be written out to storage.
	Resolve() ([]byte, error)

	// Reset clears the collector for future use.
	Reset()

	// Info reports on the current state of the collector for
	// introspection and to support schema change and payload
	// size.
	Info() CollectorInfo
}

// CollectorInfo reports on the current state of the collector and
// provides introspection into the current state of the collector for
// testing, transparency, and to support more complex collector
// features, including payload size controls and schema change
type CollectorInfo struct {
	MetricsCount int
	SampleCount  int
}
