package graphql

import (
	"context"
	"math"

	"github.com/99designs/gqlgen/graphql"
)

const (
	// testResultsComplexity is the flat cost of downloading
	// a task's test results.
	testResultsComplexity = 10
	// unboundedTaskListSize is the size assumed for a task list without a
	// positive limit.
	unboundedTaskListSize = 100
)

func newSchema(apiURL string) graphql.ExecutableSchema {
	return complexitySchema{ExecutableSchema: NewExecutableSchema(New(apiURL))}
}

type complexitySchema struct {
	graphql.ExecutableSchema
}

func (s complexitySchema) Complexity(ctx context.Context, typeName, fieldName string, childComplexity int, args map[string]any) (int, bool) {
	switch typeName + "." + fieldName {
	case "Task.tests":
		return saturatingAdd(testResultsComplexity, childComplexity), true
	case "Query.taskHistory", "Version.tasks":
		return taskListComplexity(childComplexity, args), true
	}
	return s.ExecutableSchema.Complexity(ctx, typeName, fieldName, childComplexity, args)
}

// taskListComplexity multiplies each task's complexity by options.limit.
func taskListComplexity(childComplexity int, args map[string]any) int {
	size := unboundedTaskListSize
	if limit, ok := optionsLimit(args); ok && limit > 0 {
		size = limit
	}
	return saturatingAdd(1, saturatingMultiply(childComplexity, size))
}

// optionsLimit returns options.limit from the raw args.
func optionsLimit(args map[string]any) (int, bool) {
	options, ok := args["options"].(map[string]any)
	if !ok || options["limit"] == nil {
		return 0, false
	}
	limit, err := graphql.UnmarshalInt(options["limit"])
	return limit, err == nil
}

// saturatingMultiply returns a*b capped at math.MaxInt.
func saturatingMultiply(a, b int) int {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > math.MaxInt/b {
		return math.MaxInt
	}
	return a * b
}

// saturatingAdd returns a+b capped at math.MaxInt.
func saturatingAdd(a, b int) int {
	if a > math.MaxInt-b {
		return math.MaxInt
	}
	return a + b
}
