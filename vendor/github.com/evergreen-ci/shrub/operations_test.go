package shrub

import (
	"errors"
	"fmt"
	"testing"
)

func TestWellformedOperations(t *testing.T) {
	cases := map[string]Command{
		"subprocess.exec":       CmdExec{},
		"shell.exec":            CmdExecShell{},
		"s3Copy.copy":           CmdS3Copy{},
		"s3.get":                CmdS3Get{},
		"s3.put":                CmdS3Put{CredKey: "foo", CredSecret: "bar", LocalFile: "baz"},
		"git.get_project":       CmdGetProject{},
		"attach.artifacts":      CmdAttachArtifacts{},
		"attach.results":        CmdResultsJSON{},
		"attach.xunit_results":  CmdResultsXunit{},
		"gotest.parse_files":    CmdResultsGoTest{LegacyFormat: true},
		"gotest.parse_json":     CmdResultsGoTest{JSONFormat: true},
		"archive.zip_pack":      CmdArchiveCreate{Format: ZIP},
		"archive.targz_pack":    CmdArchiveCreate{Format: TARBALL},
		"archive.zip_extract":   CmdArchiveExtract{Format: ZIP},
		"archive.targz_extract": CmdArchiveExtract{Format: TARBALL},
		"archive.auto_extract":  CmdArchiveExtract{Format: ArchiveFormat("auto")},
	}

	for name, cmd := range cases {
		t.Run("Validate_"+name, func(t *testing.T) {
			assert(t, cmd.Validate() == nil, fmt.Sprintf("validation for %T (%s)", cmd, name))
		})
		t.Run("Resolve_"+name, func(t *testing.T) {
			defer catch(t, name, "resolve")
			rcmd := cmd.Resolve()
			require(t, rcmd != nil, fmt.Sprintf("resolution for %T (%s)", cmd, name))
			assert(t, rcmd.CommandName == name, name, "not equal to", rcmd.FunctionName)
		})

	}
}

type unmarshableCmd struct {
	name string
}

func (u unmarshableCmd) Validate() error             { return nil }
func (u unmarshableCmd) Resolve() *CommandDefinition { panic("always") }
func (u unmarshableCmd) MarshalJSON() ([]byte, error) {
	return nil, errors.New("always")
}

func TestPoorlyFormedOperations(t *testing.T) {
	cases := map[string]Command{
		"s3put.empty":         CmdS3Put{},
		"s3put.nocreds":       CmdS3Put{LocalFile: "baz"},
		"s3put.nofile":        CmdS3Put{CredKey: "foo", CredSecret: "bar"},
		"s3put.nosecret":      CmdS3Put{CredKey: "foo", LocalFile: "baz"},
		"s3put.nokey":         CmdS3Put{CredSecret: "bar", LocalFile: "baz"},
		"gotest.empty":        CmdResultsGoTest{},
		"gotest.both":         CmdResultsGoTest{JSONFormat: true, LegacyFormat: true},
		"archive.create_auto": CmdArchiveCreate{Format: ArchiveFormat("auto")},
		"archive.invalid":     CmdArchiveExtract{Format: ArchiveFormat("bleh")},
	}

	for name, cmd := range cases {
		t.Run("ValidateFailsFor_"+name, func(t *testing.T) {
			assert(t, cmd.Validate() != nil, name)
		})
		t.Run("ResolvePanicsFor_"+name, func(t *testing.T) {
			defer expect(t, name)

			rcmd := cmd.Resolve()
			assert(t, rcmd == nil)
		})
	}

	t.Run("AlwaysPanicsWhenCannotMarshal", func(t *testing.T) {
		defer expect(t, "marshaling")

		res := exportCmd(unmarshableCmd{name: "sad"})
		assert(t, res == nil)
	})
}
