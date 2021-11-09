package buildlogger

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		opts   GetOptions
		hasErr bool
	}{
		{
			name: "InvalidCedarOpts",
			opts: GetOptions{
				TaskID: "task",
			},
			hasErr: true,
		},
		{
			name: "MissingIDAndTaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
			},
			hasErr: true,
		},
		{
			name: "IDAndTaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				ID:     "id",
				TaskID: "task",
			},
			hasErr: true,
		},
		{
			name: "TestNameAndNoTaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				ID:       "id",
				TestName: "test",
			},
			hasErr: true,
		},
		{
			name: "GroupIDAndNoTaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				ID:      "id",
				GroupID: "group",
			},
			hasErr: true,
		},
		{
			name: "GroupIDAndMeta",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:  "task",
				GroupID: "group",
				Meta:    true,
			},
			hasErr: true,
		},
		{
			name: "ID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				ID: "id",
			},
		},
		{
			name: "IDAndMeta",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				ID: "id",
			},
		},
		{
			name: "TaskID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID: "task",
			},
		},
		{
			name: "TaskIDAndMeta",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID: "task",
				Meta:   true,
			},
		},
		{
			name: "TestName",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:   "task",
				TestName: "test",
				Meta:     true,
			},
		},
		{
			name: "TestNameAndMeta",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:   "task",
				TestName: "test",
				Meta:     true,
			},
		},
		{
			name: "TaskIDAndGroupID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:  "task",
				GroupID: "group",
			},
		},
		{
			name: "TestNameAndGroupID",
			opts: GetOptions{
				Cedar: timber.GetOptions{
					BaseURL: "https://url.com",
				},
				TaskID:   "task",
				TestName: "test",
				GroupID:  "group",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.opts.Validate()
			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Run("ID", func(t *testing.T) {
		opts := GetOptions{
			Cedar: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			ID: "id/1",
		}
		t.Run("Logs", func(t *testing.T) {
			expectedURL := fmt.Sprintf(
				"%s/rest/v1/buildlogger/%s?paginate=true",
				opts.Cedar.BaseURL,
				url.PathEscape(opts.ID),
			)
			assert.Equal(t, expectedURL, opts.parse())
		})
		t.Run("Meta", func(t *testing.T) {
			opts.Meta = true
			expectedURL := fmt.Sprintf(
				"%s/rest/v1/buildlogger/%s/meta",
				opts.Cedar.BaseURL,
				url.PathEscape(opts.ID),
			)
			assert.Equal(t, expectedURL, opts.parse())
		})
	})
	t.Run("TaskID", func(t *testing.T) {
		opts := GetOptions{
			Cedar: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:        "task?",
			Execution:     utility.ToIntPtr(3),
			Start:         time.Now().Add(-time.Hour),
			End:           time.Now(),
			ProcessName:   "proc/1",
			Tags:          []string{"tag1?", "tag/2", "tag3"},
			PrintTime:     true,
			PrintPriority: true,
			Tail:          100,
			Limit:         1000,
		}
		t.Run("Logs", func(t *testing.T) {
			expectedURL := fmt.Sprintf(
				"%s/rest/v1/buildlogger/task_id/%s?execution=3&start=%s&end=%s&proc_name=%s&tags=%s&tags=%s&tags=tag3&print_time=true&print_priority=true&n=100&limit=1000",
				opts.Cedar.BaseURL,
				url.PathEscape(opts.TaskID),
				opts.Start.Format(time.RFC3339),
				opts.End.Format(time.RFC3339),
				url.QueryEscape(opts.ProcessName),
				url.QueryEscape(opts.Tags[0]),
				url.QueryEscape(opts.Tags[1]),
			)
			assert.Equal(t, expectedURL, opts.parse())
		})
		t.Run("Meta", func(t *testing.T) {
			opts.Meta = true
			expectedURL := fmt.Sprintf(
				"%s/rest/v1/buildlogger/task_id/%s/meta?execution=3&start=%s&end=%s&tags=%s&tags=%s&tags=tag3",
				opts.Cedar.BaseURL,
				url.PathEscape(opts.TaskID),
				opts.Start.Format(time.RFC3339),
				opts.End.Format(time.RFC3339),
				url.QueryEscape(opts.Tags[0]),
				url.QueryEscape(opts.Tags[1]),
			)
			assert.Equal(t, expectedURL, opts.parse())
		})
	})
	t.Run("TestName", func(t *testing.T) {
		opts := GetOptions{
			Cedar: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:   "task?",
			TestName: "test/1",
		}
		t.Run("Logs", func(t *testing.T) {
			expectedURL := fmt.Sprintf(
				"%s/rest/v1/buildlogger/test_name/%s/%s?paginate=true",
				opts.Cedar.BaseURL,
				url.PathEscape(opts.TaskID),
				url.PathEscape(opts.TestName),
			)
			assert.Equal(t, expectedURL, opts.parse())
		})
		t.Run("Meta", func(t *testing.T) {
			opts.Meta = true
			expectedURL := fmt.Sprintf(
				"%s/rest/v1/buildlogger/test_name/%s/%s/meta",
				opts.Cedar.BaseURL,
				url.PathEscape(opts.TaskID),
				url.PathEscape(opts.TestName),
			)
			assert.Equal(t, expectedURL, opts.parse())
		})
	})
	t.Run("TaskIDAndGroupID", func(t *testing.T) {
		opts := GetOptions{
			Cedar: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:  "task?",
			GroupID: "group/group/group",
		}
		expectedURL := fmt.Sprintf(
			"%s/rest/v1/buildlogger/task_id/%s/group/%s?paginate=true",
			opts.Cedar.BaseURL,
			url.PathEscape(opts.TaskID),
			url.PathEscape(opts.GroupID),
		)
		assert.Equal(t, expectedURL, opts.parse())
	})
	t.Run("TestNameAndGroupID", func(t *testing.T) {
		opts := GetOptions{
			Cedar: timber.GetOptions{
				BaseURL: "https://cedar.mongodb.com",
			},
			TaskID:   "task?",
			TestName: "test/?1",
			GroupID:  "group/group/group",
		}
		expectedURL := fmt.Sprintf(
			"%s/rest/v1/buildlogger/test_name/%s/%s/group/%s?paginate=true",
			opts.Cedar.BaseURL,
			url.PathEscape(opts.TaskID),
			url.PathEscape(opts.TestName),
			url.PathEscape(opts.GroupID),
		)
		assert.Equal(t, expectedURL, opts.parse())
	})
}
