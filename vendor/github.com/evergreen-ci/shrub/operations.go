package shrub

import (
	"encoding/json"
	"errors"
	"fmt"
)

////////////////////////////////////////////////////////////////////////
//
// Specific Command Implementations

func exportCmd(cmd Command) map[string]interface{} {
	if err := cmd.Validate(); err != nil {
		panic(err)
	}

	jsonStruct, err := json.Marshal(cmd)
	if err == nil {
		out := map[string]interface{}{}
		if err = json.Unmarshal(jsonStruct, &out); err == nil {
			return out
		}
	}

	panic(err)
}

type CmdExec struct {
	Background       bool   `json:"background"`
	Silent           bool   `json:"silent"`
	ContinueOnError  bool   `json:"continue_on_err"`
	SystemLog        bool   `json:"system_log"`
	CombineOuutput   bool   `json:"redirect_standard_error_to_output"`
	IgnoreStdError   bool   `json:"ignore_standard_error"`
	IgnoreStdOut     bool   `json:"ignore_standard_out"`
	KeepEmptyArgs    bool   `json:"keep_empty_args"`
	WorkingDirectory string `json:"working_dir"`
	Command          string
	Binary           string
	Args             []string
	Env              map[string]string
}

func (c CmdExec) Name() string    { return "subprocess.exec" }
func (c CmdExec) Validate() error { return nil }
func (c CmdExec) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func subprocessExecFactory() Command { return CmdExec{} }

type CmdExecShell struct {
	Background       bool   `json:"background"`
	Silent           bool   `json:"silent"`
	ContinueOnError  bool   `json:"continue_on_err"`
	SystemLog        bool   `json:"system_log"`
	CombineOuutput   bool   `json:"redirect_standard_error_to_output"`
	IgnoreStdError   bool   `json:"ignore_standard_error"`
	IgnoreStdOut     bool   `json:"ignore_standard_out"`
	WorkingDirectory string `json:"working_dir"`
	Script           string `json:"script"`
}

func (c CmdExecShell) Name() string    { return "shell.exec" }
func (c CmdExecShell) Validate() error { return nil }
func (c CmdExecShell) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func shellExecFactory() Command { return CmdExecShell{} }

type CmdS3Put struct {
	Optional               bool     `json:"optional"`
	LocalFile              string   `json:"local_file"`
	LocalFileIncludeFilter []string `json:"local_files_include_filter"`
	Bucket                 string   `json:"bucket"`
	RemoteFile             string   `json:"remote_file"`
	DisplayName            string   `json:"display_name"`
	ContentType            string   `json:"content_type"`
	CredKey                string   `json:"aws_key"`
	CredSecret             string   `json:"aws_secret"`
	Permissions            string   `json:"permissions"`
	Visibility             string   `json:"visibility"`
	BuildVariants          []string `json:"build_variants"`
}

func (c CmdS3Put) Name() string { return "s3.put" }
func (c CmdS3Put) Validate() error {
	switch {
	case c.CredKey == "", c.CredSecret == "":
		return errors.New("must specify aws credentials")
	case c.LocalFile == "" && len(c.LocalFileIncludeFilter) == 0:
		return errors.New("must specify a local file to upload")
	default:
		return nil
	}
}
func (c CmdS3Put) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3PutFactory() Command { return CmdS3Put{} }

type CmdS3Get struct {
	AWSKey        string   `json:"aws_key"`
	AWSSecret     string   `json:"aws_secret"`
	RemoteFile    string   `json:"remote_file"`
	Bucket        string   `json:"bucket"`
	LocalFile     string   `json:"local_file"`
	ExtractTo     string   `json:"extract_to"`
	BuildVariants []string `json:"build_variants"`
}

func (c CmdS3Get) Name() string    { return "s3.get" }
func (c CmdS3Get) Validate() error { return nil }
func (c CmdS3Get) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3GetFactory() Command { return CmdS3Get{} }

type CmdS3Copy struct {
	AWSKey    string `json:"aws_key"`
	AWSSecret string `json:"aws_secret"`
	Files     []struct {
		Optional      bool     `json:"optional"`
		DisplayName   string   `json:"display_name"`
		BuildVariants []string `json:"build_variants"`
		Source        struct {
			Bucket string `json:"bucket"`
			Path   string `json:"path"`
		} `json:"source"`
		Destination struct {
			Bucket string `json:"bucket"`
			Path   string `json:"path"`
		} `json:"destination"`
	} `json:"s3_copy_files"`
}

func (c CmdS3Copy) Name() string    { return "s3Copy.copy" }
func (c CmdS3Copy) Validate() error { return nil }
func (c CmdS3Copy) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3CopyFactory() Command { return CmdS3Copy{} }

type CmdGetProject struct {
	Token     string            `json:"token"`
	Directory string            `json:"directory"`
	Revisions map[string]string `json:"revisions"`
}

func (c CmdGetProject) Name() string    { return "git.get_project" }
func (c CmdGetProject) Validate() error { return nil }
func (c CmdGetProject) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func getProjectFactory() Command { return CmdGetProject{} }

type CmdResultsJSON struct {
	File string `json:"file_location"`
}

func (c CmdResultsJSON) Name() string    { return "attach.results" }
func (c CmdResultsJSON) Validate() error { return nil }
func (c CmdResultsJSON) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func jsonResultsFactory() Command { return CmdResultsJSON{} }

type CmdResultsXunit struct {
	File  string   `json:"file"`
	Files []string `json:"files"`
}

func (c CmdResultsXunit) Name() string    { return "attach.xunit_results" }
func (c CmdResultsXunit) Validate() error { return nil }
func (c CmdResultsXunit) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func xunitResultsFactory() Command { return CmdResultsXunit{} }

type CmdResultsGoTest struct {
	JSONFormat   bool `json:"-"`
	LegacyFormat bool `json:"-"`
}

func (c CmdResultsGoTest) Name() string { return "gotest.parse_json" }
func (c CmdResultsGoTest) Validate() error {
	if c.JSONFormat == c.LegacyFormat {
		return errors.New("invalid format for gotest operation")
	}

	return nil
}
func (c CmdResultsGoTest) Resolve() *CommandDefinition {
	if c.JSONFormat {
		return &CommandDefinition{
			CommandName: c.Name(),
			Params:      exportCmd(c),
		}
	}

	return &CommandDefinition{
		CommandName: "gotest.parse_files",
		Params:      exportCmd(c),
	}
}
func goTestResultsFactory() Command { return CmdResultsGoTest{} }

type ArchiveFormat string

const (
	ZIP     ArchiveFormat = "zip"
	TARBALL               = "tarball"
)

func (f ArchiveFormat) Validate() error {
	switch f {
	case ZIP, TARBALL:
		return nil
	default:
		return fmt.Errorf("'%s' is not a valid archive format", f)
	}
}

func (f ArchiveFormat) createCmdName() string {
	switch f {
	case ZIP:
		return "archive.zip_pack"
	case TARBALL:
		return "archive.targz_pack"
	default:
		panic(f.Validate())
	}
}

func (f ArchiveFormat) extractCmdName() string {
	switch f {
	case ZIP:
		return "archive.zip_extract"
	case TARBALL:
		return "archive.targz_extract"
	case "auto":
		return "archive.auto_extract"
	default:
		panic(f.Validate())
	}

}

type CmdArchiveCreate struct {
	Format    ArchiveFormat `json:"-"`
	Target    string        `json:"target"`
	SourceDir string        `json:"source_dir"`
	Include   []string      `json:"include"`
	Exclude   []string      `json:"exclude_files"`
}

func (c CmdArchiveCreate) Name() string    { return c.Format.createCmdName() }
func (c CmdArchiveCreate) Validate() error { return c.Format.Validate() }
func (c CmdArchiveCreate) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

type CmdArchiveExtract struct {
	Format  ArchiveFormat `json:"-"`
	Path    string        `json:"path"`
	Target  string        `json:"destination"`
	Exclude []string      `json:"exclude_files"`
}

func (c CmdArchiveExtract) Name() string { return c.Format.extractCmdName() }
func (c CmdArchiveExtract) Validate() error {
	err := c.Format.Validate()
	if err != nil && c.Format != "auto" {
		return err
	}

	return nil

}
func (c CmdArchiveExtract) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func archiveCreateZipFactory() Command      { return CmdArchiveCreate{Format: ZIP} }
func archiveCreateTarballFactory() Command  { return CmdArchiveCreate{Format: TARBALL} }
func archiveExtractZipFactory() Command     { return CmdArchiveExtract{Format: ZIP} }
func archiveExtractTarballFactory() Command { return CmdArchiveExtract{Format: TARBALL} }
func archiveExtractAutoFactory() Command    { return CmdArchiveExtract{Format: "auto"} }

type CmdAttachArtifacts struct {
	Optional bool     `json:"optional"`
	Files    []string `json:"files"`
}

func (c CmdAttachArtifacts) Name() string    { return "attach.artifacts" }
func (c CmdAttachArtifacts) Validate() error { return nil }
func (c CmdAttachArtifacts) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func attachArtifactsFactory() Command { return CmdAttachArtifacts{} }
