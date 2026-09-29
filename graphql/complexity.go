package graphql

import (
	"context"
	"math"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	// testResultsComplexity is the flat cost of downloading
	// a task's test results.
	testResultsComplexity = 1
	// unboundedTaskListSize is the size assumed for a task list without a
	// positive limit.
	unboundedTaskListSize = 100
)

// calculateComplexity returns gqlgen's default complexity plus
// testResultsComplexity for each time the operation resolves Task.tests.
func calculateComplexity(ctx context.Context, es graphql.ExecutableSchema, op *ast.OperationDefinition, vars map[string]any) int {
	testResultsCalls := countTestResultsCalls(op.SelectionSet, vars)
	return saturatingAdd(complexity.Calculate(ctx, es, op, vars), saturatingMultiply(testResultsComplexity, testResultsCalls))
}

// countTestResultsCalls returns the number of Task.tests fields in the
// selection set, multiplied by the size of any task lists containing them.
func countTestResultsCalls(selectionSet ast.SelectionSet, vars map[string]any) int {
	count := 0
	for _, selection := range selectionSet {
		switch s := selection.(type) {
		case *ast.Field:
			if s.ObjectDefinition == nil {
				continue
			}
			n := countTestResultsCalls(s.SelectionSet, vars)
			switch s.ObjectDefinition.Name + "." + s.Name {
			case "Task.tests":
				n = saturatingAdd(n, 1)
			case "Query.taskHistory", "Version.tasks":
				n = saturatingMultiply(n, taskListSize(s.ArgumentMap(vars)))
			}
			count = saturatingAdd(count, n)
		case *ast.FragmentSpread:
			if s.Definition != nil {
				count = saturatingAdd(count, countTestResultsCalls(s.Definition.SelectionSet, vars))
			}
		case *ast.InlineFragment:
			count = saturatingAdd(count, countTestResultsCalls(s.SelectionSet, vars))
		}
	}
	return count
}

// taskListSize returns options.limit from the args, or unboundedTaskListSize
// if it is not positive.
func taskListSize(args map[string]any) int {
	options, ok := args["options"].(map[string]any)
	if !ok || options["limit"] == nil {
		return unboundedTaskListSize
	}
	limit, err := graphql.UnmarshalInt(options["limit"])
	if err != nil || limit <= 0 {
		return unboundedTaskListSize
	}
	return limit
}

// complexityLimit replaces gqlgen's extension.ComplexityLimit to reject
// operations using calculateComplexity.
type complexityLimit struct {
	limit  int
	schema graphql.ExecutableSchema
}

func newComplexityLimit(limit int) *complexityLimit {
	return &complexityLimit{limit: limit}
}

// ExtensionName matches gqlgen's so that tracing reads the complexity stats.
func (c *complexityLimit) ExtensionName() string {
	return "ComplexityLimit"
}

func (c *complexityLimit) Validate(schema graphql.ExecutableSchema) error {
	c.schema = schema
	return nil
}

func (c *complexityLimit) MutateOperationContext(ctx context.Context, opCtx *graphql.OperationContext) *gqlerror.Error {
	op := opCtx.Doc.Operations.ForName(opCtx.OperationName)
	score := calculateComplexity(ctx, c.schema, op, opCtx.Variables)
	opCtx.Stats.SetExtension(c.ExtensionName(), &extension.ComplexityStats{
		Complexity:      score,
		ComplexityLimit: c.limit,
	})
	if score > c.limit {
		err := gqlerror.Errorf("operation has complexity %d, which exceeds the limit of %d", score, c.limit)
		errcode.Set(err, "COMPLEXITY_LIMIT_EXCEEDED")
		return err
	}
	return nil
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
