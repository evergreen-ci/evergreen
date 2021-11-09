package pail

// this is just a writer that does nothing, it is implemented for when
// gridfs and local buckets are set with dryRun to true.
type mockWriteCloser struct{}

// these functions do not do anything
func (m *mockWriteCloser) Write(p []byte) (n int, err error) { return len(p), nil }
func (m *mockWriteCloser) Close() error                      { return nil }
