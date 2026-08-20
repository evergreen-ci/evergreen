package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testProjectID    = "project"
	testTaskID       = "task"
	testKeyVar       = "artifact_aws_key"
	testSecretVar    = "artifact_aws_secret"
	testSettingsKey  = "project_settings_aws_key"
	testSettingsSecr = "project_settings_aws_secret"
)

func setupArtifactCredentials(t *testing.T, settings ArtifactCredentialSettings) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, ProjectVarsCollection,
		fakeparameter.Collection, task.Collection, evergreen.ConfigCollection))

	pRef := ProjectRef{Id: testProjectID, ArtifactCredentials: settings}
	require.NoError(t, pRef.Insert(t.Context()))

	vars := ProjectVars{Id: testProjectID, Vars: map[string]string{
		testKeyVar:       "AKIAFAKEVARKEY",
		testSecretVar:    "fake-var-secret",
		testSettingsKey:  "AKIAFAKESETTINGSKEY",
		testSettingsSecr: "fake-settings-secret",
	}}
	_, err := vars.Upsert(t.Context())
	require.NoError(t, err)

	tsk := task.Task{Id: testTaskID, Project: testProjectID, Version: "version"}
	require.NoError(t, tsk.Insert(t.Context()))
}

func TestArtifactCredentialResolver(t *testing.T) {
	staticFile := artifact.File{
		Name: "Binaries", Bucket: "bucket", FileKey: "key", Visibility: artifact.Signed,
		AWSKey: "AKIAFAKESTOREDKEY", AWSSecret: "fake-stored-secret",
	}
	varNameFile := staticFile
	varNameFile.AWSKeyVarName, varNameFile.AWSSecretVarName = testKeyVar, testSecretVar

	unresolvableFile := staticFile
	unresolvableFile.AWSKeyVarName, unresolvableFile.AWSSecretVarName = "nonexistent", "also_nonexistent"

	staticSettings := ArtifactCredentialSettings{AWSKeyVarName: testSettingsKey, AWSSecretVarName: testSettingsSecr}

	// A nil expectation means the resolver found no source, so presigning falls back
	// to the credentials stored on the artifact.
	for name, testCase := range map[string]struct {
		settings ArtifactCredentialSettings
		file     artifact.File
		expected *artifact.Credentials
		hasErr   bool
	}{
		"VarNamesOnTheArtifactBeatProjectSettingsVarNames": {
			settings: staticSettings,
			file:     varNameFile,
			expected: &artifact.Credentials{AWSKey: "AKIAFAKEVARKEY", AWSSecret: "fake-var-secret"},
		},
		"ProjectSettingsVarNamesResolveArtifactsWithoutTheirOwn": {
			settings: staticSettings,
			file:     staticFile,
			expected: &artifact.Credentials{AWSKey: "AKIAFAKESETTINGSKEY", AWSSecret: "fake-settings-secret"},
		},
		"NoConfiguredSourceResolvesToNothing": {
			file: staticFile,
		},
		"UnresolvableVarNameErrors": {
			file:   unresolvableFile,
			hasErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			setupArtifactCredentials(t, testCase.settings)

			creds, err := NewArtifactCredentialResolver(testTaskID)(t.Context(), testCase.file)
			if testCase.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, testCase.expected, creds)
		})
	}

	t.Run("DisablingFlagStopsResolutionEntirely", func(t *testing.T) {
		setupArtifactCredentials(t, staticSettings)
		flags := evergreen.ServiceFlags{LiveArtifactCredentialsDisabled: true}
		require.NoError(t, flags.Set(t.Context()))

		creds, err := NewArtifactCredentialResolver(testTaskID)(t.Context(), staticFile)
		require.NoError(t, err)
		assert.Nil(t, creds)
	})

	t.Run("ResolutionIsCachedAcrossFilesOnTheSameTask", func(t *testing.T) {
		setupArtifactCredentials(t, staticSettings)
		resolver := NewArtifactCredentialResolver(testTaskID)
		creds, err := resolver(t.Context(), staticFile)
		require.NoError(t, err)

		// A second lookup would fail, so resolving again proves it was cached.
		require.NoError(t, db.ClearCollections(ProjectRefCollection))

		cachedCreds, err := resolver(t.Context(), staticFile)
		require.NoError(t, err)
		assert.Equal(t, creds, cachedCreds)
	})
}
