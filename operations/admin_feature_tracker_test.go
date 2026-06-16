package operations

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleConfig exercises display tasks, task groups, and generate.tasks (the
// latter reached indirectly through a function), plus a couple of commands.
const sampleConfig = `
functions:
  gen:
    - command: generate.tasks
      params:
        files:
          - gen.json
tasks:
  - name: compile
    commands:
      - command: shell.exec
        params: {script: "echo build"}
  - name: generator
    commands:
      - func: gen
  - name: unit
    commands:
      - command: subprocess.exec
        params: {binary: "echo"}
      - command: gotest.parse_files
        params:
          files:
            - "*.suite"
task_groups:
  - name: my_tg
    max_hosts: 2
    tasks:
      - unit
buildvariants:
  - name: ubuntu
    display_name: Ubuntu
    run_on: [ubuntu2004]
    tasks:
      - name: compile
      - name: generator
      - name: unit
    display_tasks:
      - name: bundle
        execution_tasks:
          - compile
          - unit
`

func TestDetectorsAgainstSampleConfig(t *testing.T) {
	var project model.Project
	pp, err := model.LoadProjectInto(context.Background(), []byte(sampleConfig), nil, "sample", &project)
	require.NoError(t, err)

	counts := map[string]int{}
	for _, d := range ftDetectors() {
		counts[d.Name] = d.Detect(&project, pp)
	}

	for name, expected := range map[string]int{
		"display_tasks":      1,
		"task_groups":        1,
		"generate_tasks":     1,
		"subprocess_exec":    1,
		"shell_exec":         1,
		"gotest_parse_files": 1,
		"modules":            0,
		"matrices":           0,
		"cache_save":         0,
		"host_create":        0,
	} {
		assert.Equal(t, expected, counts[name], "detector %q", name)
	}
}

// matrixConfig defines a single matrix build variant. Matrices only exist on
// the parser project; translation expands them into concrete variants, so the
// detector must read the parser project rather than the translated project.
const matrixConfig = `
axes:
  - id: os
    values:
      - id: linux
        display_name: Linux
        run_on: ubuntu2004
tasks:
  - name: t1
    commands:
      - command: shell.exec
        params: {script: "echo hi"}
buildvariants:
  - matrix_name: build_matrix
    matrix_spec: {os: "*"}
    tasks:
      - name: t1
`

func TestMatrixDetectorReadsParserProject(t *testing.T) {
	var matrixDet ftDetector
	for _, d := range ftDetectors() {
		if d.Name == "matrices" {
			matrixDet = d
		}
	}
	require.NotEmpty(t, matrixDet.Name, "matrices detector should be registered")

	var project model.Project
	pp, err := model.LoadProjectInto(context.Background(), []byte(matrixConfig), nil, "sample", &project)
	require.NoError(t, err)

	// The matrix is detectable on the parser project but expanded away on the
	// translated project, so detecting against the project alone returns 0.
	assert.Equal(t, 1, matrixDet.Detect(&project, pp))
	assert.Equal(t, 0, matrixDet.Detect(&project, nil))
}

// nonTaskBlockConfig invokes commands from every block that is not a task's
// own command list: the pre/post/timeout blocks and a task group's
// setup_group/setup_task/teardown_task. gotest.parse_files is reached
// indirectly through a function from both the post block and the task group
// teardown, mirroring the real evergreen.yml layout. Counting only task
// commands (the old behavior) misses all of these.
const nonTaskBlockConfig = `
functions:
  attach:
    - command: gotest.parse_files
      params:
        files:
          - "*.suite"
pre:
  - command: s3.put
post:
  - func: attach
timeout:
  - command: shell.exec
    params: {script: "echo timeout"}
tasks:
  - name: unit
    commands:
      - command: subprocess.exec
        params: {binary: "echo"}
task_groups:
  - name: tg
    max_hosts: 2
    setup_group:
      - command: manifest.load
    setup_task:
      - command: ec2.assume_role
    teardown_task:
      - func: attach
    tasks:
      - unit
buildvariants:
  - name: ubuntu
    display_name: Ubuntu
    run_on: [ubuntu2004]
    tasks:
      - name: unit
`

func TestCommandDetectorCountsNonTaskBlocks(t *testing.T) {
	var project model.Project
	pp, err := model.LoadProjectInto(context.Background(), []byte(nonTaskBlockConfig), nil, "sample", &project)
	require.NoError(t, err)

	counts := map[string]int{}
	for _, d := range ftDetectors() {
		counts[d.Name] = d.Detect(&project, pp)
	}

	for name, expected := range map[string]int{
		// Reached through the attach function from both the post block and the
		// task group teardown_task.
		"gotest_parse_files": 2,
		"s3_put":             1, // pre block.
		"shell_exec":         1, // timeout block.
		"subprocess_exec":    1, // task command.
		"manifest_load":      1, // task group setup_group.
		"ec2_assume_role":    1, // task group setup_task.
	} {
		assert.Equal(t, expected, counts[name], "detector %q", name)
	}
}

func TestComputeAdoptionCountsProjectsNotInvocations(t *testing.T) {
	dets := []ftDetector{{Name: "feat", Detect: func(*model.Project, *model.ParserProject) int { return 0 }}}
	results := []ftResult{
		{Project: "a", Counts: map[string]int{"feat": 5}},
		{Project: "b", Counts: map[string]int{"feat": 0}},
		{Project: "c", Counts: map[string]int{"feat": 2}},
	}

	adopt := ftComputeAdoption(dets, results)
	require.Len(t, adopt, 1)
	// Two of three projects use the feature, regardless of invocation counts.
	assert.Equal(t, 2, adopt[0].Using)
	assert.Equal(t, 3, adopt[0].Total)
	assert.Equal(t, "66.7%", adopt[0].Percent)
}
