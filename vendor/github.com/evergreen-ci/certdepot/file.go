package certdepot

import (
	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
)

type fileDepot struct {
	*depot.FileDepot
	opts DepotOptions
}

// NewFileDepot creates a FileDepot wrapped with certdepot.Depot.
func NewFileDepot(dir string) (Depot, error) {
	dt, err := depot.NewFileDepot(dir)
	if err != nil {
		return nil, errors.WithStack(err)

	}
	return &fileDepot{FileDepot: dt}, nil
}

// MakeFileDepot constructs a file-based depot implementation and
// allows users to specify options for the default CA name and
// expiration time.
func MakeFileDepot(dir string, opts DepotOptions) (Depot, error) {
	dt, err := NewFileDepot(dir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fd, ok := dt.(*fileDepot)
	if !ok {
		return nil, errors.New("internal error constructing depot")
	}

	fd.opts = opts
	return fd, nil
}

func (fd *fileDepot) Save(name string, creds *Credentials) error { return depotSave(fd, name, creds) }
func (fd *fileDepot) Find(name string) (*Credentials, error)     { return depotFind(fd, name, fd.opts) }
func (fd *fileDepot) Generate(name string) (*Credentials, error) {
	return depotGenerateDefault(fd, name, fd.opts)
}

func (fd *fileDepot) GenerateWithOptions(opts CertificateOptions) (*Credentials, error) {
	return depotGenerate(fd, opts.CommonName, fd.opts, opts)
}
