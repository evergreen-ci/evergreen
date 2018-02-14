package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratedTasksFromYAML(t *testing.T) {
	jsonTestDoc := `
{
    "functions": {
        "echo-hi": {
            "command": "shell.exec",
            "params": {
                "script": "echo hi"
            }
        },
        "echo-bye": [
            {
                "command": "shell.exec",
                "params": {
                    "script": "echo bye"
                }
            },
            {
                "command": "shell.exec",
                "params": {
                    "script": "echo bye again"
                }
            }
        ],
    },
    "tasks": [
        {
            "commands": [
                {
                    "command": "git.get_project",
                    "params": {
                        "directory": "src"
                    }
                },
                {
                    "func": "echo-hi"
                }
            ],
            "name": "test"
        }
    ],
    "buildvariants": [
        {
            "tasks": [
                {
                    "name": "test"
                }
            ],
            "display_name": "Ubuntu 16.04 Small",
            "run_on": [
                "ubuntu1604-test"
            ],
            "name": "first"
        },
        {
            "tasks": [
                {
                    "name": "test"
                }
            ],
            "display_name": "Ubuntu 16.04 Large",
            "run_on": [
                "ubuntu1604-build"
            ],
            "name": "second"
        }

    ]
}

`
	assert := assert.New(t)
	g, err := GeneratedTasksFromJSON([]byte(jsonTestDoc))
	assert.NotNil(g)
	assert.Nil(err)

	assert.Len(g.Functions, 2)
	assert.Contains(g.Functions, "echo-hi")
	assert.Equal("shell.exec", g.Functions["echo-hi"].List()[0].Command)
	assert.Equal("echo hi", g.Functions["echo-hi"].List()[0].Params["script"])
	assert.Equal("echo bye", g.Functions["echo-bye"].List()[0].Params["script"])
	assert.Equal("echo bye again", g.Functions["echo-bye"].List()[1].Params["script"])

	assert.Len(g.Tasks, 1)
	assert.Equal("git.get_project", g.Tasks[0].Commands[0].Command)
	assert.Equal("src", g.Tasks[0].Commands[0].Params["directory"])
	assert.Equal("echo-hi", g.Tasks[0].Commands[1].Function)
	assert.Equal("test", g.Tasks[0].Name)

	assert.Len(g.BuildVariants, 2)
	assert.Equal("Ubuntu 16.04 Large", g.BuildVariants[1].DisplayName)
	assert.Equal("second", g.BuildVariants[1].Name)
	assert.Equal("test", g.BuildVariants[1].Tasks[0].Name)
	assert.Equal("ubuntu1604-build", g.BuildVariants[1].RunOn[0])
}
