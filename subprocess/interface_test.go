package subprocess

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterfaceCompliance(t *testing.T) {
	assert := assert.New(t) // nolint

	assert.Implements((*Command)(nil), &LocalCommand{})
	assert.Implements((*Command)(nil), &RemoteCommand{})
	assert.Implements((*Command)(nil), &LocalExec{})
	assert.Implements((*Command)(nil), &ScpCommand{})
}

func TestOutputOptions(t *testing.T) {
	assert := assert.New(t) // nolint

	opts := OutputOptions{}
	assert.Error(opts.Validate())

	stdout := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})

	opts.Output = stdout
	opts.Error = stderr
	assert.NoError(opts.Validate())

	// invalid if both streams are the same
	opts.Output = stderr
	assert.Error(opts.Validate())
	opts.Output = stdout
	assert.NoError(opts.Validate())

	// if the redirection and suppression options don't make
	// sense, validate should error, for stderr
	opts.SuppressError = true
	assert.Error(opts.Validate())
	opts.Error = nil
	assert.NoError(opts.Validate())
	opts.SendOutputToError = true
	assert.Error(opts.Validate())
	opts.SuppressError = false
	opts.Error = stderr
	assert.NoError(opts.Validate())
	opts.SendOutputToError = false
	assert.NoError(opts.Validate())

	// the same but for stdout
	opts.SuppressOutput = true
	assert.Error(opts.Validate())
	opts.Output = nil
	assert.NoError(opts.Validate())
	opts.SendErrorToOutput = true
	assert.Error(opts.Validate())
	opts.SuppressOutput = false
	opts.Output = stdout
	assert.NoError(opts.Validate())
	opts.SuppressOutput = false
	assert.NoError(opts.Validate())
}
