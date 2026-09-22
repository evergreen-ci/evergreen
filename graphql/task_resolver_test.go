package graphql

import (
	"context"
	"testing"

	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestTaskResolverErrors(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(task.Collection, distro.Collection))

	validDistro := distro.Distro{
		Id:      "valid-distro",
		Aliases: []string{"valid-distro-alias"},
	}
	require.NoError(t, validDistro.Insert(ctx))

	warningDistro := distro.Distro{
		Id:          "warning-distro",
		WarningNote: "being deprecated",
	}
	require.NoError(t, warningDistro.Insert(ctx))

	r := &taskResolver{}

	t.Run("NoErrorsForValidPrimaryDistro", func(t *testing.T) {
		obj := &restModel.APITask{
			Id:       utility.ToStringPtr("task-with-valid-distro"),
			DistroId: utility.ToStringPtr(validDistro.Id),
		}
		errs, err := r.Errors(ctx, obj)
		require.NoError(t, err)
		assert.Empty(t, errs)
	})

	t.Run("NoErrorsForValidPrimaryDistroReferencedByAlias", func(t *testing.T) {
		obj := &restModel.APITask{
			Id:       utility.ToStringPtr("task-with-distro-alias"),
			DistroId: utility.ToStringPtr("valid-distro-alias"),
		}
		errs, err := r.Errors(ctx, obj)
		require.NoError(t, err)
		assert.Empty(t, errs)
	})

	t.Run("ReportsInvalidPrimaryDistro", func(t *testing.T) {
		obj := &restModel.APITask{
			Id:       utility.ToStringPtr("task-with-invalid-distro"),
			DistroId: utility.ToStringPtr("nonexistent-distro"),
		}
		errs, err := r.Errors(ctx, obj)
		require.NoError(t, err)
		require.Len(t, errs, 1)
		assert.Equal(t, distro.DistroNotFoundMessage("nonexistent-distro"), errs[0])
	})

	t.Run("ReportsWarningForDistroWithWarningNote", func(t *testing.T) {
		obj := &restModel.APITask{
			Id:       utility.ToStringPtr("task-with-warning-distro"),
			DistroId: utility.ToStringPtr(warningDistro.Id),
		}
		errs, err := r.Errors(ctx, obj)
		require.NoError(t, err)
		expectedMsg, hasWarning := warningDistro.WarningNoteMessage()
		require.True(t, hasWarning)
		assert.Equal(t, []string{expectedMsg}, errs)
	})
}

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
