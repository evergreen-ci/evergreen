package model

import (
	"sort"

	"github.com/evergreen-ci/utility"
)

// APIVariantQuarantineStatus represents the manual-quarantine status for every
// known test of every known task in a build variant.
type APIVariantQuarantineStatus struct {
	ProjectIdentifier *string                  `json:"project_identifier"`
	BuildVariant      *string                  `json:"build_variant"`
	Tasks             []APITaskQuarantineEntry `json:"tasks"`
}

// APITaskQuarantineEntry is the per-task quarantine view within a build
// variant.
type APITaskQuarantineEntry struct {
	TaskName *string                  `json:"task_name"`
	Tests    []APITestQuarantineEntry `json:"tests"`
}

// APITestQuarantineEntry is the per-test quarantine view within a task.
type APITestQuarantineEntry struct {
	TestName              *string `json:"test_name"`
	IsManuallyQuarantined bool    `json:"is_manually_quarantined"`
}

type VariantQuarantineStatusBuildArgs struct {
	ProjectIdentifier string
	BuildVariant      string
	Tasks             map[string]map[string]bool
}

// BuildFromService populates the variant quarantine status from the given
// args. Tasks and tests are sorted alphabetically so output is stable.
func (s *APIVariantQuarantineStatus) BuildFromService(args VariantQuarantineStatusBuildArgs) {
	s.ProjectIdentifier = utility.ToStringPtr(args.ProjectIdentifier)
	s.BuildVariant = utility.ToStringPtr(args.BuildVariant)

	taskNames := make([]string, 0, len(args.Tasks))
	for taskName := range args.Tasks {
		taskNames = append(taskNames, taskName)
	}
	sort.Strings(taskNames)

	s.Tasks = make([]APITaskQuarantineEntry, 0, len(taskNames))
	for _, taskName := range taskNames {
		tests := args.Tasks[taskName]
		testNames := make([]string, 0, len(tests))
		for testName := range tests {
			testNames = append(testNames, testName)
		}
		sort.Strings(testNames)

		entries := make([]APITestQuarantineEntry, 0, len(testNames))
		for _, testName := range testNames {
			entries = append(entries, APITestQuarantineEntry{
				TestName:              utility.ToStringPtr(testName),
				IsManuallyQuarantined: tests[testName],
			})
		}
		s.Tasks = append(s.Tasks, APITaskQuarantineEntry{
			TaskName: utility.ToStringPtr(taskName),
			Tests:    entries,
		})
	}
}
