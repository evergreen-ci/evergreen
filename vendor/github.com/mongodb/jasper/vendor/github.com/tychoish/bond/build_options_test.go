package bond

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildOptionsValidation(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(BuildOptions{
		Target:  "foo",
		Arch:    "foo",
		Edition: "foo",
	}.Validate())

	assert.Error(BuildOptions{}.Validate())

	assert.Error(BuildOptions{
		Target:  "",
		Arch:    "foo",
		Edition: "foo",
	}.Validate())

	assert.Error(BuildOptions{
		Target:  "foo",
		Arch:    "",
		Edition: "foo",
	}.Validate())

	assert.Error(BuildOptions{
		Target:  "foo",
		Arch:    "foo",
		Edition: "",
	}.Validate())

	opts := BuildOptions{"foo", "foo", "foo", true}
	info := opts.GetBuildInfo("bar")
	assert.Exactly(info.Options, opts)
	assert.Exactly("bar", info.Version)
}
