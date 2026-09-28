package graphql

import (
	"context"
	"math"

	"github.com/99designs/gqlgen/graphql"
)

const (
	// unboundedListComplexityMultiplier is the multiplier applied to a list
	// field whose request does not set a positive limit. Such requests return
	// every matching item, so they are scored as if they requested a large
	// page.
	unboundedListComplexityMultiplier = 100
	// defaultTaskHistoryLimit is the default value of TaskHistoryOpts.limit in
	// the schema.
	defaultTaskHistoryLimit = 50
)

// NewSchema returns the executable GraphQL schema with custom complexity
// scoring for list fields whose cost scales with the requested limit.
func NewSchema(apiURL string) graphql.ExecutableSchema {
	return complexitySchema{ExecutableSchema: NewExecutableSchema(New(apiURL))}
}

// complexitySchema overrides the complexity of specific fields. It reads
// limits directly from the raw arguments rather than using gqlgen's Config
// complexity functions, because the generated argument unmarshalling runs input
// directives such as @requireProjectAccess, which query the database and
// silently fall back to the default complexity when they fail.
type complexitySchema struct {
	graphql.ExecutableSchema
}

func (s complexitySchema) Complexity(ctx context.Context, typeName, fieldName string, childComplexity int, args map[string]any) (int, bool) {
	switch typeName + "." + fieldName {
	case "Task.tests":
		return listComplexity(childComplexity, inputLimit(args, "opts", 0)), true
	case "Query.taskHistory":
		return listComplexity(childComplexity, inputLimit(args, "options", defaultTaskHistoryLimit)), true
	}
	return s.ExecutableSchema.Complexity(ctx, typeName, fieldName, childComplexity, args)
}

// inputLimit returns the "limit" field of the input object argument argName,
// or defaultLimit if it is not set.
func inputLimit(args map[string]any, argName string, defaultLimit int) int {
	input, ok := args[argName].(map[string]any)
	if !ok {
		return defaultLimit
	}
	rawLimit, ok := input["limit"]
	if !ok || rawLimit == nil {
		return defaultLimit
	}
	limit, err := graphql.UnmarshalInt(rawLimit)
	if err != nil {
		return defaultLimit
	}
	return limit
}

// listComplexity scores a list field as 1 plus the complexity of its children
// multiplied by the number of items requested. A non-positive limit means the
// list is unbounded. The multiplier is applied to the field's entire selection
// set because gqlgen does not expose the complexity of individual child
// fields.
func listComplexity(childComplexity, limit int) int {
	multiplier := unboundedListComplexityMultiplier
	if limit > 0 {
		multiplier = limit
	}
	return saturatingAdd(1, saturatingMultiply(childComplexity, multiplier))
}

// saturatingMultiply returns a*b for non-negative operands, capped at
// math.MaxInt so that a very large requested limit cannot overflow into a
// small or negative score.
func saturatingMultiply(a, b int) int {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > math.MaxInt/b {
		return math.MaxInt
	}
	return a * b
}

// saturatingAdd returns a+b for non-negative operands, capped at math.MaxInt.
func saturatingAdd(a, b int) int {
	if a > math.MaxInt-b {
		return math.MaxInt
	}
	return a + b
}
