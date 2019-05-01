package dependency

import "os"

const createTypeName = "create-file"

type createsFile struct {
	FileName string   `bson:"file_name" json:"file_name" yaml:"file_name"`
	T        TypeInfo `bson:"type" json:"type" yaml:"type"`
	JobEdges
}

func makeCreatesFile() *createsFile {
	return &createsFile{
		T: TypeInfo{
			Name:    createTypeName,
			Version: 0,
		},
		JobEdges: NewJobEdges(),
	}
}

// NewCreatesFile constructs a dependency manager object to support
// tasks that are tasks are ready to run if a specific file doesn't
// exist.
func NewCreatesFile(name string) Manager {
	c := makeCreatesFile()
	c.FileName = name

	return c
}

// State returns Ready if the dependent file does not exist or is not
// specified, and Passed if the file *does* exist. Jobs with Ready
// states should be executed, while those with Passed states should be
// a no-op.
func (d *createsFile) State() State {
	if d.FileName == "" {
		return Ready
	}

	if _, err := os.Stat(d.FileName); os.IsNotExist(err) {
		return Ready
	}

	return Passed
}

// Type returns the type information on the "creates-file" Manager
// implementation.
func (d *createsFile) Type() TypeInfo {
	return d.T
}
