package testutil

// GolangMainSuccess returns the contents of a Golang main script that will
// succeed when run.
func GolangMainSuccess() string {
	return `package main; import "os"; func main() { os.Exit(0) }`
}

// GolangMainFail returns the contents of a Golang main script that will fail
// when run.
func GolangMainFail() string {
	return `package main; import "os"; func main() { os.Exit(42) }`
}

// GolangTestSuccess returns the contents of a Golang test that will succeed
// when run.
func GolangTestSuccess() string {
	return `package main; import "testing"; func TestMain(t *testing.T) { t.Skip() }`
}

// GolangTestFail returns the contents of a Golang test that will fail when runk.
func GolangTestFail() string {
	return `package main; import "testing"; func TestMain(t *testing.T) { t.FailNow() }`
}
