package command

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// macSign is a command to sign and notarize macOS artifacts.
type macSign struct {
	// KeyId and Secret are the credentials for
	// authenticating into the macOS signing and notarization service.
	KeyId  string `mapstructure:"key_id" plugin:"expand"`
	Secret string `mapstructure:"secret" plugin:"expand"`

	// ServiceUrl is the url of the macOS signing and notarization service
	ServiceUrl string `mapstructure:"service_url" plugin:"expand"`

	// ClientBinary is the path to the macOS signing and notarization service client.
	// If empty, the default location (/usr/local/bin/macnotary) will be used.
	ClientBinary string `mapstructure:"client_binary" plugin:"expand"`

	// LocalZipFile is the local filepath to the zip file the user wishes to
	// sign. It should contain the artifacts that need to be signed.
	LocalZipFile string `mapstructure:"local_zip_file" plugin:"expand"`

	// OutputZipFile is the local filepath to the zip file the service outputs
	// It will contain the list of artifacts that are signed by the server.
	OutputZipFile string `mapstructure:"output_zip_file" plugin:"expand"`

	// ArtifactType is the type of artifact(s) that need to be signed.
	// Currently supported list: app, binary.
	ArtifactType string `mapstructure:"artifact_type" plugin:"expand"`

	// EntitlementsFilePath is the local filepath to the entitlements file that the user
	// wishes to execute the signing process with. This is optional.
	EntitlementsFilePath string `mapstructure:"entitlements_file" plugin:"expand"`

	// Verify determines if the signature (or notarization) should be verified.
	// Verification is only supported on macOS. It is optional, default value if false.
	Verify bool `mapstructure:"verify"`

	// Notarize determines if the file should also be notarized after signing.
	Notarize bool `mapstructure:"notarize"`

	// BundleId is the bundle ID of the artifact used during notarization.
	// This is mandatory if notarization is requested.
	BundleId string `mapstructure:"bundle_id"  plugin:"expand"`

	// BuildVariants stores a list of build variants to run the command for.
	// If the list is empty, it runs for all build variants.
	BuildVariants []string `mapstructure:"build_variants"`

	// WorkingDir sets the current working directory.
	WorkingDir string `mapstructure:"working_directory" plugin:"expand"`

	base
}

func macSignFactory() Command         { return &macSign{} }
func (macSign *macSign) Name() string { return "mac.sign" }

// macSign-specific implementation of ParseParams.
func (macSign *macSign) ParseParams(params map[string]interface{}) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: macSign,
	})
	if err != nil {
		return errors.Wrap(err, "constructing mapstructure decoder")
	}

	if err := decoder.Decode(params); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	return macSign.validate()
}

func (macSign *macSign) validate() error {
	catcher := grip.NewSimpleCatcher()

	// make sure the command params are valid
	if macSign.KeyId == "" {
		catcher.New("key ID cannot be blank")
	}
	if macSign.Secret == "" {
		catcher.New("secret cannot be blank")
	}
	if macSign.LocalZipFile == "" {
		catcher.New("local zip file cannot be blank")
	}
	if macSign.OutputZipFile == "" {
		catcher.New("output zip file cannot be blank")
	}
	if macSign.ServiceUrl == "" {
		catcher.New("service URL cannot be blank")
	}
	if runtime.GOOS != "darwin" {
		// do not fail, just set verifying to false.
		macSign.Verify = false
	}
	if !(macSign.ArtifactType == "" || macSign.ArtifactType == "binary" || macSign.ArtifactType == "app") {
		catcher.New("artifact needs to be either empty, 'binary' or 'app'")
	}
	if macSign.Notarize && macSign.BundleId == "" {
		catcher.New("if notarization is requested, bundle ID cannot be blank")
	}

	return catcher.Resolve()
}

// Apply the expansions from the relevant task config
// to all appropriate fields of the macSign.
func (macSign *macSign) expandParams(conf *internal.TaskConfig) error {
	var err error
	if macSign.WorkingDir == "" {
		macSign.WorkingDir = conf.WorkDir
	}

	if err = util.ExpandValues(macSign, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	if !filepath.IsAbs(macSign.LocalZipFile) {
		macSign.LocalZipFile = filepath.Join(macSign.WorkingDir, macSign.LocalZipFile)
	}
	if !filepath.IsAbs(macSign.OutputZipFile) {
		macSign.OutputZipFile = filepath.Join(macSign.WorkingDir, macSign.OutputZipFile)
	}
	if macSign.EntitlementsFilePath != "" && !filepath.IsAbs(macSign.EntitlementsFilePath) {
		macSign.EntitlementsFilePath = filepath.Join(macSign.WorkingDir, macSign.EntitlementsFilePath)
	}
	if macSign.ClientBinary != "" && !filepath.IsAbs(macSign.ClientBinary) {
		macSign.ClientBinary = filepath.Join(macSign.WorkingDir, macSign.ClientBinary)
	}

	// setting default client binary if not given
	if macSign.ClientBinary == "" {
		macSign.ClientBinary = "/usr/local/bin/macnotary"
	}

	return nil
}

func (macSign *macSign) shouldRunForVariant(buildVariantName string) bool {
	//If no buildvariant filter, run for everything
	if len(macSign.BuildVariants) == 0 {
		return true
	}

	//Only run if the buildvariant specified appears in the list.
	return utility.StringSliceContains(macSign.BuildVariants, buildVariantName)
}

// Implementation of Execute. Expands the parameters,
// and then execute macOS signing and/or notarization.
func (macSign *macSign) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := macSign.expandParams(conf); err != nil {
		return errors.WithStack(err)
	}

	if err := createEnclosingDirectoryIfNeeded(macSign.WorkingDir); err != nil {
		return errors.Wrap(err, "making working directory")
	}

	if !macSign.shouldRunForVariant(conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping macsign of local file '%s' for variant '%s'",
			macSign.LocalZipFile, conf.BuildVariant.Name)
		return nil
	}

	signMode := "sign"
	if macSign.Notarize {
		signMode = "notarizeAndSign"
	}
	args := []string{"-f", macSign.LocalZipFile,
		"-k", macSign.KeyId,
		"-s", macSign.Secret,
		"-u", macSign.ServiceUrl,
		"-m", signMode,
		"-o", macSign.OutputZipFile,
	}
	if macSign.EntitlementsFilePath != "" {
		args = append(args, "-e", macSign.EntitlementsFilePath)
	}
	if macSign.ArtifactType != "" {
		args = append(args, "-t", macSign.ArtifactType)
	}
	if macSign.BundleId != "" {
		args = append(args, "-b", macSign.BundleId)
	}
	if macSign.Verify {
		args = append(args, "--verify")
	}

	cmd := exec.Command(macSign.ClientBinary, args...)

	stdout, err := cmd.CombinedOutput()
	output := string(stdout)

	if len(output) != 0 {
		logger.Task().Info(output)
	}

	if err != nil {

		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return errors.Wrapf(err, "unexpected error on OS '%s' arch '%s'", runtime.GOOS, runtime.GOARCH)
		}

		if exitErr.ExitCode() != 0 {
			return errors.Errorf("non-zero exit code %d", exitErr.ExitCode())
		}
	}

	logger.Task().Infof("Artifact - file '%s' signed (and/or notarized) and new file created: '%s'.", macSign.LocalZipFile, macSign.OutputZipFile)

	return nil
}
