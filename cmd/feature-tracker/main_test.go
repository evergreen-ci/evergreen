package main

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
	_, err := model.LoadProjectInto(context.Background(), []byte(sampleConfig), nil, "sample", &project)
	require.NoError(t, err)

	counts := map[string]int{}
	for _, d := range detectors() {
		counts[d.Name] = d.Detect(&project)
	}

	for name, expected := range map[string]int{
		"display_tasks":   1,
		"task_groups":     1,
		"generate_tasks":  1,
		"subprocess_exec": 1,
		"shell_exec":      1,
		"modules":         0,
		"cache_save":      0,
		"host_create":     0,
	} {
		assert.Equal(t, expected, counts[name], "detector %q", name)
	}
}

func TestComputeAdoptionCountsProjectsNotInvocations(t *testing.T) {
	dets := []Detector{{Name: "feat", Detect: func(*model.Project) int { return 0 }}}
	results := []result{
		{Project: "a", Counts: map[string]int{"feat": 5}},
		{Project: "b", Counts: map[string]int{"feat": 0}},
		{Project: "c", Counts: map[string]int{"feat": 2}},
	}

	adopt := computeAdoption(dets, results)
	require.Len(t, adopt, 1)
	// Two of three projects use the feature, regardless of invocation counts.
	assert.Equal(t, 2, adopt[0].Using)
	assert.Equal(t, 3, adopt[0].Total)
	assert.Equal(t, "66.7%", adopt[0].Percent)
}
