package graphql

import (
	"context"
	"testing"

	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestShouldDecorateTestQuarantineStatus(t *testing.T) {
	// The tests field resolves to a TaskTestResult whose immediate children are testResults,
	// totalTestCount, and filteredTestCount. isManuallyQuarantined is only reachable one level
	// deeper, nested under testResults.
	ctxWithTestResultsSelection := func(testResultsSelection ast.SelectionSet) context.Context {
		ctx := gqlgen.WithOperationContext(context.Background(), &gqlgen.OperationContext{})
		return gqlgen.WithFieldContext(ctx, &gqlgen.FieldContext{
			Field: gqlgen.CollectedField{
				Field: &ast.Field{Name: "tests", Alias: "tests"},
				Selections: ast.SelectionSet{
					&ast.Field{Name: "testResults", Alias: "testResults", SelectionSet: testResultsSelection},
					&ast.Field{Name: "totalTestCount", Alias: "totalTestCount"},
				},
			},
		})
	}

	t.Run("TrueWhenIsManuallyQuarantinedNestedUnderTestResults", func(t *testing.T) {
		ctx := ctxWithTestResultsSelection(ast.SelectionSet{
			&ast.Field{Name: "testFile", Alias: "testFile"},
			&ast.Field{Name: "isManuallyQuarantined", Alias: "isManuallyQuarantined"},
		})
		assert.True(t, shouldDecorateTestQuarantineStatus(ctx))
	})

	t.Run("FalseWhenIsManuallyQuarantinedNotSelected", func(t *testing.T) {
		ctx := ctxWithTestResultsSelection(ast.SelectionSet{
			&ast.Field{Name: "testFile", Alias: "testFile"},
		})
		assert.False(t, shouldDecorateTestQuarantineStatus(ctx))
	})

	t.Run("TrueWithoutOperationContext", func(t *testing.T) {
		assert.True(t, shouldDecorateTestQuarantineStatus(context.Background()))
	})
}
