package subprocess

import (
	"bytes"
	"testing"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
)

func TestInterfaceCompliance(t *testing.T) {
	assert := assert.New(t)

	assert.Implements((*Command)(nil), &localCmd{})
	assert.Implements((*Command)(nil), &remoteCmd{})
	assert.Implements((*Command)(nil), &localExec{})
	assert.Implements((*Command)(nil), &scpCommand{})
}

func TestOutputOptions(t *testing.T) {
	assert := assert.New(t)

	opts := OutputOptions{}
	assert.NoError(opts.Validate())

	stdout := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})

	opts.Output = stdout
	opts.Error = stderr
	assert.NoError(opts.Validate())

	opts.Output = stderr // valid if they're the same
	assert.NoError(opts.Validate())
	opts.Output = stdout // reset

	// if the redirection and suppression options don't make
	// sense, validate should error, for stderr
	opts.SuppressError = true
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
	opts.Output = nil
	assert.NoError(opts.Validate())
	opts.SendErrorToOutput = true
	assert.Error(opts.Validate())
	opts.SuppressOutput = false
	opts.Output = stdout
	assert.NoError(opts.Validate())
	opts.SuppressOutput = false
	assert.NoError(opts.Validate())

	// but should be valid if you suppress both
	opts = OutputOptions{SuppressError: true, SuppressOutput: true}
	assert.NoError(opts.Validate())
}

func TestOutputOptionsIntegrationTableTest(t *testing.T) {
	// these are integration tests to test various output configurations used throughout the code base
	assert := assert.New(t)

	buf := &bytes.Buffer{}
	shouldFail := []OutputOptions{
		{Output: buf, SendOutputToError: true},
		{SendOutputToError: true, SuppressError: true},
		{SendErrorToOutput: true, SuppressOutput: true},
		{SendErrorToOutput: true, SendOutputToError: true},
	}

	shouldPass := []OutputOptions{
		{Output: buf, Error: buf},
		{SuppressError: true, SuppressOutput: true},
		{Output: buf, SendErrorToOutput: true},
		{Output: &util.CappedWriter{Buffer: buf, MaxBytes: 1024 * 1024}, SendErrorToOutput: true},
	}

	for idx, opt := range shouldFail {
		assert.Error(opt.Validate(), "%d: %+v", idx, opt)
		grip.Debug(opt)
	}

	for idx, opt := range shouldPass {
		assert.NoError(opt.Validate(), "%d: %+v", idx, opt)
		grip.Debug(opt)
	}

}
