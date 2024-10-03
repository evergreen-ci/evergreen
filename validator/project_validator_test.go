package validator

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	_ "github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestProjectErrorValidators(t *testing.T) {
	// projectErrorValidators have some restrictions and conventions that they must follow:
	// 1. They must return an error explicitly.
	// 2. They must not return any other type of ValidationError level.
	testProjectValidatorsFunctions(t, projectErrorValidators, func(t *testing.T, funcBodies map[string]*ast.BlockStmt, funcName string) {
		assert.True(t, variablesInFunction(funcBodies, funcName, []string{"Error"}, map[string]bool{}), "ProjectErrorValidators should return at least one Error")
		assert.False(t, variablesInFunction(funcBodies, funcName, []string{"Warning", "Notice"}, map[string]bool{}), "ProjectErrorValidators should never use Warnings or Notices")
	})
}

func TestProjectWarningValidators(t *testing.T) {
	// projectWarningValidators must only return Warning or Notice.
	testProjectValidatorsFunctions(t, projectWarningValidators, func(t *testing.T, funcBodies map[string]*ast.BlockStmt, funcName string) {
		assert.False(t, variablesInFunction(funcBodies, funcName, []string{"Error"}, map[string]bool{}), "ProjectWarningValidators should never use Error")
		assert.True(t, variablesInFunction(funcBodies, funcName, []string{"Warning", "Notice"}, map[string]bool{}), "ProjectWarningValidators return at least one Warning or Notice")
	})
}

// testProjectValidatorsFunctions parses through all the given project validators and runs the given test function on each one.
func testProjectValidatorsFunctions(t *testing.T, projectValidators []projectValidator, test func(t *testing.T, funcBodies map[string]*ast.BlockStmt, funcName string)) {
	node, err := parser.ParseFile(token.NewFileSet(), "project_validator.go", nil, parser.AllErrors)
	require.NoError(t, err)
	funcBodies := make(map[string]*ast.BlockStmt)
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			funcBodies[fn.Name.Name] = fn.Body
		}
		return true
	})

	// projectErrorValidators have some restrictions and conventions that they must follow:
	// 1. They must return an error explicitly.
	// 2. They must not return any other type of ValidationError level.
	for _, validator := range projectValidators {
		funcPtr := runtime.FuncForPC(reflect.ValueOf(validator).Pointer())
		funcName := funcPtr.Name()[strings.LastIndex(funcPtr.Name(), ".")+1:]

		t.Run(funcName, func(t *testing.T) {
			test(t, funcBodies, funcName)
		})
	}
}

// variablesInFunction recursively checks if the given variables are used in a function or any of the functions it calls.
func variablesInFunction(funcBodies map[string]*ast.BlockStmt, funcName string, variableNames []string, visited map[string]bool) bool {
	if visited[funcName] {
		return false
	}
	visited[funcName] = true

	body, exists := funcBodies[funcName]
	if !exists {
		return false
	}

	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		// Check if the variable is used directly in the function.
		if ident, ok := n.(*ast.Ident); ok && utility.StringSliceContains(variableNames, ident.Name) {
			// If the object is nil, that means it is a call expression.
			// e.g. (error).Error() would match the identifier for the
			// call expression if the variable = "Error".
			if ident.Obj == nil {
				return true
			}
			found = true
			return false
		}
		// Check if the variable is used in a function call.
		if callExpr, ok := n.(*ast.CallExpr); ok {
			// If this is a call expression, get the function identifier.
			if funIdent, ok := callExpr.Fun.(*ast.Ident); ok {
				if variablesInFunction(funcBodies, funIdent.Name, variableNames, visited) {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func TestValidateStatusesForTaskDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("SucceedsWithTaskDependingOnTaskInSpecificBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    variant: bv2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("IgnoresDuplicateDependencies", func(t *testing.T) {
		// Duplicate dependencies are allowed because when the project is
		// translated, it consolidates all the duplicates.
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
  - name: t2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("ErrorsWithInvalidDependencyStatus", func(t *testing.T) {
		// Duplicate dependencies are allowed because when the project is
		// translated, it consolidates all the duplicates.
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    status: foobar
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Error, errs[0].Level)
		assert.Contains(t, errs[0].Message, "invalid dependency status 'foobar' for task 't2'")
	})
	t.Run("AllowsDependenciesOnSameTaskInDifferentBuildVariants", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    variant: bv1
  - name: t2
    variant: bv2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskImplicitlyDependingOnTaskInSameBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskImplicitlyDependingOnTaskGroupTaskInSameBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

task_groups:
- name: tg1
  tasks:
  - t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: tg1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskGroupTaskImplicitlyDependingOnTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

task_groups:
- name: tg1
  tasks:
  - t1

buildvariants:
- name: bv1
  tasks:
  - name: tg1
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithBuildVariantTaskDependingOnTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
    depends_on:
    - name: t2
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithBuildVariantTaskDependingOnTaskAndOverridingTaskDependencyDefinition", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t3
- name: t2
- name: t3

buildvariants:
- name: bv1
  tasks:
  - name: t1
    depends_on:
    - name: t2
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithBuildVariantDependingOnTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
- name: t2

buildvariants:
- name: bv1
  depends_on:
  - name: t2
    variant: bv2
  tasks:
  - name: t1
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskDependingOnAllTasksAndAllVariants", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: "*"
    variant: "*"

buildvariants:
- name: bv1
  tasks:
  - name: t1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskDependingOnAllTasksInSpecificVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: "*"
    variant: bv2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskDependingOnSpecificTaskInAllBuildVariants", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    variant: "*"
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := validateStatusesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
}

func TestCheckReferencesForTaskDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("WarnsWithTaskDependingOnNonexistentTaskInSpecificBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    variant: bv2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
- name: bv2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv2', but it was not found")
	})
	t.Run("IgnoresDuplicateDependencies", func(t *testing.T) {
		// Duplicate dependencies are allowed because when the project is
		// translated, it consolidates all the duplicates.
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
  - name: t2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("AllowsDependenciesOnSameTaskInDifferentBuildVariants", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    variant: bv1
  - name: t2
    variant: bv2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskImplicitlyDependingOnTaskInSameBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithTaskImplicitlyDependingOnNonexistentTaskInSameBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv1', but it was not found")
	})
	t.Run("SucceedsWithTaskImplicitlyDependingOnTaskGroupTaskInSameBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

task_groups:
- name: tg1
  tasks:
  - t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
  - name: tg1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithTaskImplicitlyDependingOnNonexistentTaskGroupTaskInSameBuildVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

task_groups:
- name: tg1
  tasks:
  - t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv1', but it was not found")
	})
	t.Run("SucceedsWithTaskGroupTaskImplicitlyDependingOnTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

task_groups:
- name: tg1
  tasks:
  - t1

buildvariants:
- name: bv1
  tasks:
  - name: tg1
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithTaskGroupTaskImplicitlyDependingOnNonexistentTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2

task_groups:
- name: tg1
  tasks:
  - t1

buildvariants:
- name: bv1
  tasks:
  - name: tg1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv1', but it was not found")
	})
	t.Run("SucceedsWithBuildVariantTaskDependingOnTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
    depends_on:
    - name: t2
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithBuildVariantTaskDependingOnNonexistentTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
    depends_on:
    - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv1', but it was not found")
	})
	t.Run("SucceedsWithBuildVariantTaskDependingOnTaskAndOverridingTaskDependencyDefinition", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t3
- name: t2
- name: t3

buildvariants:
- name: bv1
  tasks:
  - name: t1
    depends_on:
    - name: t2
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithBuildVariantTaskDependingOnNonexistentTaskAndOverridingTaskDependencyDefinition", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
- name: t2
- name: t3

buildvariants:
- name: bv1
  tasks:
  - name: t1
    depends_on:
    - name: t3
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't3' in build variant 'bv1', but it was not found")
	})
	t.Run("SucceedsWithBuildVariantDependingOnTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
- name: t2

buildvariants:
- name: bv1
  depends_on:
  - name: t2
    variant: bv2
  tasks:
  - name: t1
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithBuildVariantDependingOnNonexistentTask", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
- name: t2

buildvariants:
- name: bv1
  depends_on:
  - name: t2
    variant: bv2
  tasks:
  - name: t1
- name: bv2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv2', but it was not found")
	})
	t.Run("SucceedsWithTaskDependingOnAllTasksAndAllVariants", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: "*"
    variant: "*"

buildvariants:
- name: bv1
  tasks:
  - name: t1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsWithTaskDependingOnAllTasksInSpecificVariant", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: "*"
    variant: bv2
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithTaskDependingOnAllTasksInSpecificNonexistentVariant", func(t *testing.T) {
		p := model.Project{
			Tasks: []model.ProjectTask{
				{
					Name: "t1",
					DependsOn: []model.TaskUnitDependency{
						{
							Name:    model.AllDependencies,
							Variant: "bv2",
						},
					},
				},
				{Name: "t2"},
			},
			BuildVariants: []model.BuildVariant{
				{
					Name: "bv1",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "t1",
							Variant: "bv1",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    model.AllDependencies,
									Variant: "bv2",
								},
							},
						},
					},
				},
			},
		}
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on '*' tasks in build variant 'bv2', but the build variant was not found")
	})
	t.Run("SucceedsWithTaskDependingOnSpecificTaskInAllBuildVariants", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    variant: "*"
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
- name: bv2
  tasks:
  - name: t2
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithTaskDependingOnSpecificTaskInAllBuildVariantsButNoneMatch", func(t *testing.T) {
		projYAML := `
tasks:
- name: t1
  depends_on:
  - name: t2
    variant: "*"
- name: t2

buildvariants:
- name: bv1
  tasks:
  - name: t1
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in '*' build variants, but no build variant contains that task")
	})
	t.Run("WarnsWithTaskDependingOnNonexistentTask", func(t *testing.T) {
		p := model.Project{
			Tasks: []model.ProjectTask{
				{
					Name: "t1",
					DependsOn: []model.TaskUnitDependency{
						{
							Name: "t2",
						},
					},
				},
			},
			BuildVariants: []model.BuildVariant{
				{
					Name: "bv1",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "t1",
							Variant: "bv1",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "t2",
									Variant: "bv1",
								},
							},
						},
					},
				},
			},
		}
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv1', but it was not found")
	})
	t.Run("WarnsWithTaskDependingOnNonexistentVariant", func(t *testing.T) {
		p := model.Project{
			Tasks: []model.ProjectTask{
				{
					Name: "t1",
					DependsOn: []model.TaskUnitDependency{
						{
							Name:    "t2",
							Variant: "bv2",
						},
					},
				},
			},
			BuildVariants: []model.BuildVariant{
				{
					Name: "bv1",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "t1",
							Variant: "bv1",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "t2",
									Variant: "bv2",
								},
							},
						},
					},
				},
			},
		}
		errs := checkReferencesForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, errs[0].Message, "task 't1' in build variant 'bv1' depends on task 't2' in build variant 'bv2', but it was not found")
	})
}

func TestCheckRequestersForTaskDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("SucceedsWithNilDependencySetting", func(t *testing.T) {
		projYAML := `
tasks:
  - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

  - name: task
    patch_only: true
    depends_on:
      - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"
buildvariants:
  - name: ubuntu2204
    display_name: Ubuntu 22.04
    run_on:
      - localhost
    tasks:
      - name: "task"
      - name: "dep"
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkRequestersForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsAtTaskLevelMatch", func(t *testing.T) {
		projYAML := `
tasks:
  - name: dep
    patch_only: true
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

  - name: task
    patch_only: true
    depends_on:
      - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"
buildvariants:
  - name: ubuntu2204
    display_name: Ubuntu 22.04
    run_on:
      - localhost
    tasks:
      - name: "task"
      - name: "dep"
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkRequestersForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsAtBuildVariantLevelMatch", func(t *testing.T) {
		projYAML := `
tasks:
  - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

  - name: task
    patch_only: true
    depends_on:
      - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"
buildvariants:
  - name: ubuntu2204
    patch_only: true
    display_name: Ubuntu 22.04
    run_on:
      - localhost
    tasks:
      - name: "task"
      - name: "dep"
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkRequestersForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("SucceedsAtBuildVariantTaskLevelMatch", func(t *testing.T) {
		projYAML := `
tasks:
  - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

  - name: task
    patch_only: true
    depends_on:
      - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"
buildvariants:
  - name: ubuntu2204
    display_name: Ubuntu 22.04
    run_on:
      - localhost
    tasks:
      - name: "task"
      - name: "dep"
        patch_only: true
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkRequestersForTaskDependencies(&p)

		assert.Empty(t, errs)
	})
	t.Run("WarnsWithTaskLevelConflict", func(t *testing.T) {
		projYAML := `
tasks:
  - name: dep
    patch_only: true
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"
  - name: task
    patch_only: false
    depends_on:
      - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

buildvariants:
  - name: ubuntu2204
    display_name: Ubuntu 22.04
    run_on:
      - localhost
    tasks:
      - name: "task"
      - name: "dep"
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkRequestersForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, errs[0].Level, Warning)
		assert.Contains(t, errs[0].Message, "'task' depends on patch-only task 'dep'")
	})
	t.Run("WarnsWithVariantLevelConflict", func(t *testing.T) {
		projYAML := `
tasks:
  - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

  - name: task
    patch_only: false
    depends_on:
      - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

buildvariants:
  - name: ubuntu2204
    patch_only: true
    display_name: Ubuntu 22.04
    run_on:
      - localhost
    tasks:
      - name: "task"
      - name: "dep"
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkRequestersForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, errs[0].Level, Warning)
		assert.Contains(t, errs[0].Message, "'task' depends on patch-only task 'dep'")
	})
	t.Run("WarnsWithBuildVariantTaskLevelConflict", func(t *testing.T) {
		projYAML := `
tasks:
  - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

  - name: task
    patch_only: false
    depends_on:
      - name: dep
    commands:
      - command: shell.exec
        params:
          shell: bash
          script: |
            echo "hi"

buildvariants:
  - name: ubuntu2204
    display_name: Ubuntu 22.04
    run_on:
      - localhost
    tasks:
      - name: "task"
      - name: "dep"
        patch_only: true
`
		var p model.Project
		_, err := model.LoadProjectInto(ctx, []byte(projYAML), nil, "", &p)
		require.NoError(t, err)
		errs := checkRequestersForTaskDependencies(&p)

		require.Len(t, errs, 1)
		assert.Equal(t, errs[0].Level, Warning)
		assert.Contains(t, errs[0].Message, "'task' depends on patch-only task 'dep'")
	})
}

func TestValidateDependencyGraph(t *testing.T) {
	Convey("When checking a project's dependency graph", t, func() {
		Convey("cycles in the dependency graph should cause error to be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:      "compile",
								Variant:   "bv",
								DependsOn: []model.TaskUnitDependency{{Name: "testOne"}},
							},
							{
								Name:      "testOne",
								Variant:   "bv",
								DependsOn: []model.TaskUnitDependency{{Name: "compile"}},
							},
							{
								Name:      "testTwo",
								Variant:   "bv",
								DependsOn: []model.TaskUnitDependency{{Name: "compile"}},
							},
						},
					},
				},
			}
			errs := validateDependencyGraph(project)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})

		Convey("task wildcard cycles in the dependency graph should return an error", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "compile",
								Variant: "bv",
							},
							{
								Name:      "testOne",
								Variant:   "bv",
								DependsOn: []model.TaskUnitDependency{{Name: "compile"}, {Name: "testTwo"}},
							},
							{
								Name:      "testTwo",
								Variant:   "bv",
								DependsOn: []model.TaskUnitDependency{{Name: model.AllDependencies}},
							},
						},
					},
				},
			}

			errs := validateDependencyGraph(project)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})

		Convey("cross-variant cycles in the dependency graph should return an error", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "compile",
								Variant: "bv1",
							},
							{
								Name:    "testOne",
								Variant: "bv1",
								DependsOn: []model.TaskUnitDependency{
									{Name: "compile"},
									{Name: "testSpecial", Variant: "bv2"},
								},
							}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "testSpecial", Variant: "bv2", DependsOn: []model.TaskUnitDependency{{Name: "testOne", Variant: "bv1"}}}},
					},
				},
			}

			errs := validateDependencyGraph(project)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})

		Convey("variant wildcard cycles in the dependency graph should return an error", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv1"},
							{Name: "testOne", Variant: "bv1", DependsOn: []model.TaskUnitDependency{
								{Name: "compile"},
								{Name: "testSpecial", Variant: "bv2"},
							}}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "testSpecial", Variant: "bv2", DependsOn: []model.TaskUnitDependency{{Name: "testOne", Variant: model.AllVariants}}},
						},
					},
					{
						Name: "bv3",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv3"},
							{Name: "testOne", Variant: "bv3", DependsOn: []model.TaskUnitDependency{
								{Name: "compile"},
								{Name: "testSpecial", Variant: "bv2"},
							}}},
					},
					{
						Name: "bv4",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv4"},
							{Name: "testOne", Variant: "bv4", DependsOn: []model.TaskUnitDependency{
								{Name: "compile"},
								{Name: "testSpecial", Variant: "bv2"},
							}}},
					},
				},
			}

			errs := validateDependencyGraph(project)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})

		Convey("cycles in a ** dependency graph should return an error", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "compile",
								Variant: "bv1",
							},
							{
								Name:    "testOne",
								Variant: "bv1",
								DependsOn: []model.TaskUnitDependency{
									{Name: "compile", Variant: model.AllVariants},
									{Name: "testTwo"},
								},
							}},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "compile",
								Variant: "bv2",
							},
							{
								Name:    "testOne",
								Variant: "bv2",
								DependsOn: []model.TaskUnitDependency{
									{Name: "compile", Variant: model.AllVariants},
									{Name: "testTwo"},
								},
							},
							{
								Name:    "testTwo",
								Variant: "bv2",
								DependsOn: []model.TaskUnitDependency{
									{Name: model.AllDependencies, Variant: model.AllVariants},
								},
							},
						},
					},
				},
			}

			errs := validateDependencyGraph(project)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})

		Convey("if any task has itself as a dependency, no error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv"},
							{Name: "testOne", Variant: "bv", DependsOn: []model.TaskUnitDependency{{Name: "testOne"}}},
						},
					},
				},
			}
			So(validateDependencyGraph(project), ShouldResemble, ValidationErrors{})
		})

		Convey("if there is no cycle in the dependency graph, no error should"+
			" be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv"},
							{Name: "testOne", Variant: "bv", DependsOn: []model.TaskUnitDependency{{Name: "compile"}}},
							{Name: "testTwo", Variant: "bv", DependsOn: []model.TaskUnitDependency{{Name: "compile"}}}},
					},
				},
			}
			So(validateDependencyGraph(project), ShouldResemble, ValidationErrors{})
		})

		Convey("if there is no cycle in the cross-variant dependency graph, no error should"+
			" be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "testOne",
								Variant: "bv1",
								DependsOn: []model.TaskUnitDependency{
									{Name: "compile", Variant: "bv2"},
								},
							},
						},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv2"},
							{
								Name:    "testSpecial",
								Variant: "bv2",
								DependsOn: []model.TaskUnitDependency{
									{Name: "compile"},
									{Name: "testOne", Variant: "bv1"}},
							},
						},
					},
				},
			}

			So(validateDependencyGraph(project), ShouldResemble, ValidationErrors{})
		})

		Convey("if there is no cycle in the * dependency graph, no error should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv1"},
							{Name: "testOne", Variant: "bv1", DependsOn: []model.TaskUnitDependency{
								{Name: "compile", Variant: model.AllVariants},
							}},
						},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv2"},
							{Name: "testTwo", Variant: "bv2", DependsOn: []model.TaskUnitDependency{
								{Name: model.AllDependencies},
							}},
						},
					},
				},
			}

			So(validateDependencyGraph(project), ShouldResemble, ValidationErrors{})
		})

		Convey("if there is no cycle in the ** dependency graph, no error should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "bv1",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv1"},
							{Name: "testOne", Variant: "bv1", DependsOn: []model.TaskUnitDependency{
								{Name: "compile", Variant: model.AllVariants},
							}},
						},
					},
					{
						Name: "bv2",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "bv2"},
							{Name: "testOne", Variant: "bv2", DependsOn: []model.TaskUnitDependency{
								{Name: "compile", Variant: model.AllVariants},
							}},
							{Name: "testTwo", Variant: "bv2", DependsOn: []model.TaskUnitDependency{
								{Name: model.AllDependencies, Variant: model.AllVariants}},
							}},
					},
				},
			}

			So(validateDependencyGraph(project), ShouldResemble, ValidationErrors{})
		})

	})
}

func TestCheckTaskRuns(t *testing.T) {
	makeProject := func() *model.Project {
		return &model.Project{
			Tasks: []model.ProjectTask{
				{
					Name: "task",
				},
			},
			BuildVariants: []model.BuildVariant{
				{
					Name: "bv",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "task", Variant: "bv"},
					},
				},
			},
		}
	}
	Convey("When a task is patchable, not patch-only, and not git-tag-only, no error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].Patchable = utility.TruePtr()
		project.BuildVariants[0].Tasks[0].PatchOnly = utility.FalsePtr()
		project.BuildVariants[0].Tasks[0].GitTagOnly = utility.FalsePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 0)
	})
	Convey("When a task is not patchable, no error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].Patchable = utility.FalsePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 0)
	})
	Convey("When a task is patch-only, no error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].PatchOnly = utility.TruePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 0)
	})
	Convey("When a task is git-tag-only, no error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].GitTagOnly = utility.TruePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 0)
	})
	Convey("When a task is not patchable and not patch-only, no error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].Patchable = utility.FalsePtr()
		project.BuildVariants[0].Tasks[0].PatchOnly = utility.FalsePtr()
	})
	Convey("When a task is not patchable and patch-only, an error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].Patchable = utility.FalsePtr()
		project.BuildVariants[0].Tasks[0].PatchOnly = utility.TruePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 1)
	})
	Convey("When a task is patchable and git-tag-only, an error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].Patchable = utility.TruePtr()
		project.BuildVariants[0].Tasks[0].GitTagOnly = utility.TruePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 1)
	})
	Convey("When a task is patch-only and git-tag-only, an error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].PatchOnly = utility.TruePtr()
		project.BuildVariants[0].Tasks[0].GitTagOnly = utility.TruePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 1)
	})
	Convey("When a task is not allowed for git tags and git-tag-only, an error should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].AllowForGitTag = utility.FalsePtr()
		project.BuildVariants[0].Tasks[0].GitTagOnly = utility.TruePtr()
		So(len(checkTaskRuns(project)), ShouldEqual, 1)
	})
	Convey("When a task is patch-only and also has allowed requesters, a warning should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].AllowedRequesters = []evergreen.UserRequester{
			evergreen.PatchVersionUserRequester,
			evergreen.RepotrackerVersionUserRequester,
		}
		project.BuildVariants[0].Tasks[0].PatchOnly = utility.TruePtr()
		errs := checkTaskRuns(project)
		So(len(errs), ShouldEqual, 1)
		So(errs[0].Level, ShouldEqual, Warning)
	})
	Convey("When a task is not patchable and also has allowed requesters, a warning should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].AllowedRequesters = []evergreen.UserRequester{
			evergreen.PatchVersionUserRequester,
			evergreen.RepotrackerVersionUserRequester,
		}
		project.BuildVariants[0].Tasks[0].Patchable = utility.FalsePtr()
		errs := checkTaskRuns(project)
		So(len(errs), ShouldEqual, 1)
		So(errs[0].Level, ShouldEqual, Warning)
	})
	Convey("When a task is git-tag-only and also has allowed requesters, a warning should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].AllowedRequesters = []evergreen.UserRequester{
			evergreen.PatchVersionUserRequester,
			evergreen.RepotrackerVersionUserRequester,
		}
		project.BuildVariants[0].Tasks[0].GitTagOnly = utility.TruePtr()
		errs := checkTaskRuns(project)
		So(len(errs), ShouldEqual, 1)
		So(errs[0].Level, ShouldEqual, Warning)
	})
	Convey("When a task is allowed for git tags and also has allowed requesters, a warning should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].AllowedRequesters = []evergreen.UserRequester{
			evergreen.GitTagUserRequester,
			evergreen.RepotrackerVersionUserRequester,
		}
		project.BuildVariants[0].Tasks[0].AllowForGitTag = utility.TruePtr()
		errs := checkTaskRuns(project)
		So(len(errs), ShouldEqual, 1)
		So(errs[0].Level, ShouldEqual, Warning)
	})
	Convey("When a task has a valid allowed requester, no warning or error should be thrown", t, func() {
		project := makeProject()
		for _, userRequester := range evergreen.AllUserRequesterTypes {
			project.BuildVariants[0].Tasks[0].AllowedRequesters = []evergreen.UserRequester{userRequester}
			errs := checkTaskRuns(project)
			So(len(errs), ShouldEqual, 0)
		}
	})
	Convey("When a task has an invalid allowed_requester, a warning should be thrown", t, func() {
		project := makeProject()
		project.BuildVariants[0].Tasks[0].AllowedRequesters = []evergreen.UserRequester{"foobar"}
		errs := checkTaskRuns(project)
		So(len(errs), ShouldEqual, 1)
		So(errs[0].Message, ShouldContainSubstring, "invalid allowed_requester")
		So(errs[0].Level, ShouldEqual, Warning)
	})
}

func TestValidateTimeoutLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	project := &model.Project{
		Tasks: []model.ProjectTask{
			{
				Name:            "task",
				ExecTimeoutSecs: 10,
			},
		},
	}

	t.Run("SucceedsWithTimeoutBelowLimit", func(t *testing.T) {
		settings := &evergreen.Settings{
			TaskLimits: evergreen.TaskLimitsConfig{
				MaxExecTimeoutSecs: 100,
			},
		}
		assert.Empty(t, validateTimeoutLimits(ctx, settings, project, &model.ProjectRef{}, false))
	})
	t.Run("FailsWithTimeoutExceedingLimit", func(t *testing.T) {
		settings := &evergreen.Settings{
			TaskLimits: evergreen.TaskLimitsConfig{
				MaxExecTimeoutSecs: 1,
			},
		}
		errs := validateTimeoutLimits(ctx, settings, project, &model.ProjectRef{}, false)
		require.Len(t, errs, 1)
		assert.Equal(t, Warning, errs[0].Level)
		assert.Contains(t, "task 'task' exec timeout (10) is too high and will be set to maximum limit (1)", errs[0].Message)
	})
	t.Run("SucceedsWithNoMaxTimeoutLimit", func(t *testing.T) {
		settings := &evergreen.Settings{}
		assert.Empty(t, validateTimeoutLimits(ctx, settings, project, &model.ProjectRef{}, false))
	})
}

func TestValidateIncludeLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	project := &model.Project{
		NumIncludes: 10,
	}

	t.Run("SucceedsWithNumberOfIncludesBelowLimit", func(t *testing.T) {
		settings := &evergreen.Settings{
			TaskLimits: evergreen.TaskLimitsConfig{
				MaxIncludesPerVersion: 100,
			},
		}
		assert.Empty(t, validateIncludeLimits(ctx, settings, project, &model.ProjectRef{}, false))
	})
	t.Run("FailsWithIncludesExceedingLimit", func(t *testing.T) {
		settings := &evergreen.Settings{
			TaskLimits: evergreen.TaskLimitsConfig{
				MaxIncludesPerVersion: 1,
			},
		}
		errs := validateIncludeLimits(ctx, settings, project, &model.ProjectRef{}, false)
		require.Len(t, errs, 1)
		assert.Equal(t, Error, errs[0].Level)
		assert.Contains(t, "project's total number of includes (10) exceeds maximum limit (1)", errs[0].Message)
	})
	t.Run("SucceedsWithNoMaxIncludesLimit", func(t *testing.T) {
		settings := &evergreen.Settings{}
		assert.Empty(t, validateIncludeLimits(ctx, settings, project, &model.ProjectRef{}, false))
	})
}

func TestValidateProjectLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	makeProjectWithDoubleNumTasks := func(numTasks int) *model.Project {
		var project model.Project
		project.BuildVariants = []model.BuildVariant{
			{
				Name: "bv1",
			},
			{
				Name: "bv2",
			},
		}

		for i := 0; i < numTasks; i++ {
			t := model.ProjectTask{
				Name: fmt.Sprintf("task-%d", i),
			}
			project.Tasks = append(project.Tasks, t)
			project.BuildVariants[0].Tasks = append(project.BuildVariants[0].Tasks, model.BuildVariantTaskUnit{
				Name:    t.Name,
				Variant: project.BuildVariants[0].Name,
			})
			project.BuildVariants[1].Tasks = append(project.BuildVariants[1].Tasks, model.BuildVariantTaskUnit{
				Name:    t.Name,
				Variant: project.BuildVariants[1].Name,
			})
		}
		return &project
	}

	t.Run("SucceedsWithNumberOfTasksBelowLimit", func(t *testing.T) {
		settings := &evergreen.Settings{
			TaskLimits: evergreen.TaskLimitsConfig{
				MaxTasksPerVersion: 10,
			},
		}
		project := makeProjectWithDoubleNumTasks(3)
		assert.Empty(t, validateProjectLimits(ctx, settings, project, &model.ProjectRef{}, false))
	})
	t.Run("FailsWithTasksExceedingLimit", func(t *testing.T) {
		settings := &evergreen.Settings{
			TaskLimits: evergreen.TaskLimitsConfig{
				MaxTasksPerVersion: 10,
			},
		}
		project := makeProjectWithDoubleNumTasks(50)
		errs := validateProjectLimits(ctx, settings, project, &model.ProjectRef{}, false)
		require.Len(t, errs, 1)
		assert.Equal(t, Error, errs[0].Level)
		assert.Contains(t, "project's total number of tasks (100) exceeds maximum limit (10)", errs[0].Message)
	})
	t.Run("SucceedsWithNoMaxTaskLimit", func(t *testing.T) {
		settings := &evergreen.Settings{}
		project := makeProjectWithDoubleNumTasks(50)
		assert.Empty(t, validateProjectLimits(ctx, settings, project, &model.ProjectRef{}, false))
	})
}

func TestValidateTaskNames(t *testing.T) {
	Convey("When a task name contains unauthorized characters, an error should be returned", t, func() {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "task|"},
				{Name: "|task"},
				{Name: "ta|sk"},
				{Name: "this is my task"},
				{Name: "task()<"},
				{Name: "task'"},
				{Name: "task{}"},
			},
		}
		validationResults := validateTaskNames(project)
		So(len(validationResults), ShouldEqual, len(project.Tasks))
	})
	Convey("When a task name is valid, no error should be returned", t, func() {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "task"},
				{Name: "unittest--[a-z]"},
				{Name: `check:sasl=Cyrus\_\u2022\_tls=LibreSSL\_\u2022\_test_mongocxx_ref=r3.9.0`},
			},
		}
		validationResults := validateTaskNames(project)
		So(len(validationResults), ShouldEqual, 0)
	})
	Convey("A warning should be returned when a task name", t, func() {
		Convey("Contains commas", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{{Name: "task,"}},
			}
			errs := checkTasks(project)
			So(len(errs), ShouldEqual, 3)
			assert.Contains(t, errs.String(), "task name 'task,' should not contain commas")
		})
		Convey("Is the same as the all-dependencies syntax", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{{Name: model.AllDependencies}},
			}
			errs := checkTasks(project)
			So(len(errs), ShouldEqual, 3)
			assert.Contains(t, errs.String(), "task should not be named '*' because it is ambiguous with the all-dependencies '*' specification")
		})
		Convey("Is 'all'", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{{Name: "all"}},
			}
			errs := checkTasks(project)
			So(len(errs), ShouldEqual, 3)
			assert.Contains(t, errs.String(), "task should not be named 'all' because it is ambiguous in task specifications for patches")
		})
	})
}

func TestCheckTasksUsed(t *testing.T) {
	t.Run("UsedInTasksAndDisplay", func(t *testing.T) {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "t1"},
				{Name: "execTask"},
			},
			BuildVariants: model.BuildVariants{
				{
					Name: "v1",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t1"},
						{Name: "execTask"},
					},
					DisplayTasks: []patch.DisplayTask{
						{Name: "dt", ExecTasks: []string{"execTask"}},
					},
				},
			},
		}
		errs := checkTaskUsage(project)
		assert.Len(t, errs, 0)
	})
	t.Run("ExecTaskNotListedWithTasks", func(t *testing.T) {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "t1"},
				{Name: "execTask"},
			},
			BuildVariants: model.BuildVariants{
				{
					Name: "v1",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t1"},
					},
					DisplayTasks: []patch.DisplayTask{
						{Name: "dt", ExecTasks: []string{"execTask"}},
					},
				},
			},
		}
		errs := checkTaskUsage(project)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, "'execTask' defined but not used")
		assert.Equal(t, Notice, errs[0].Level)
	})
	t.Run("DisabledTask", func(t *testing.T) {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "t1"},
				{Name: "t2", Disable: utility.TruePtr()},
			},
			BuildVariants: model.BuildVariants{
				{
					Name: "v1",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t1"},
					},
				},
			},
		}
		errs := checkTaskUsage(project)
		require.Len(t, errs, 0)
	})
	t.Run("UnusedTaskInDisabledVariant", func(t *testing.T) {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "t1"},
			},
			BuildVariants: model.BuildVariants{
				{
					Name: "v1",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t1"},
					},
					Disable: utility.TruePtr(),
				},
			},
		}
		errs := checkTaskUsage(project)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, "'t1' defined but not used")
		assert.Equal(t, Notice, errs[0].Level)
	})
	t.Run("UnusedTaskDisabledForVariant", func(t *testing.T) {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "t1"},
			},
			BuildVariants: model.BuildVariants{
				{
					Name: "v1",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t1", Disable: utility.TruePtr()},
					},
				},
			},
		}
		errs := checkTaskUsage(project)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, "'t1' defined but not used")
		assert.Equal(t, Notice, errs[0].Level)
	})
	t.Run("MultipleVariants", func(t *testing.T) {
		project := &model.Project{
			Tasks: []model.ProjectTask{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "t3"},
				{Name: "t4"},
			},
			BuildVariants: model.BuildVariants{
				{
					Name: "v1",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t1", Disable: utility.TruePtr()},
					},
				},
				{
					Name: "v2",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t1"},
						{Name: "t2"},
					},
				},
				{
					Name: "disabledVariant",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "t2"},
						{Name: "t3"},
					},
					Disable: utility.TruePtr(),
				},
			},
		}
		errs := checkTaskUsage(project)
		require.Len(t, errs, 2)
	})
}

func TestCheckModules(t *testing.T) {
	Convey("When validating a project's modules", t, func() {
		Convey("An error should be returned when more than one module shares the same name or is empty", func() {
			project := &model.Project{
				Modules: model.ModuleList{
					model.Module{
						Name:   "module-0",
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
					model.Module{
						Name:   "module-0",
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
					model.Module{
						Name:   "module-1",
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
					model.Module{
						Name:   "module-2",
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
					model.Module{
						Name:   "module-1",
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
					model.Module{
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
				},
			}
			So(len(checkModules(project)), ShouldEqual, 3)
		})

		Convey("An error should be returned when the module does not have a branch", func() {
			project := &model.Project{
				Modules: model.ModuleList{
					model.Module{
						Name: "module-0",
						Repo: "git@github.com:evergreen-ci/evergreen.git",
					},
					model.Module{
						Name:   "module-1",
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
					model.Module{
						Name: "module-2",
						Repo: "git@github.com:evergreen-ci/evergreen.git",
					},
				},
			}
			So(len(checkModules(project)), ShouldEqual, 2)
		})

		Convey("An error should be returned when the module's repo is empty or invalid", func() {
			project := &model.Project{
				Modules: model.ModuleList{
					model.Module{
						Name:   "module-0",
						Branch: "main",
						Owner:  "evergreen-ci",
						Repo:   "evergreen",
					},
					model.Module{ // should fail
						Name:   "module-2",
						Branch: "main",
						Owner:  "evergreen-ci",
					},
					model.Module{ // should fail
						Name:   "module-3",
						Branch: "main",
						Repo:   "evergreen",
					},
					model.Module{
						Name:   "module-4",
						Branch: "main",
						Repo:   "git@github.com:evergreen-ci/evergreen.git",
					},
				},
			}
			So(len(checkModules(project)), ShouldEqual, 2)
		})
	})
}

func TestValidateBVNames(t *testing.T) {
	Convey("When validating a project's build variants' names", t, func() {
		Convey("if any variant has a duplicate entry, an error should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux"},
					{Name: "linux"},
				},
			}
			validationResults := validateBVNames(project)
			So(validationResults, ShouldNotResemble, ValidationErrors{})
			So(len(validationResults), ShouldEqual, 3)
			So(validationResults[0].Level, ShouldEqual, Error)
		})

		Convey("if any variant has warnings from translating, an warning should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux", TranslationWarnings: []string{"this is a warning"}},
				},
			}
			validationResults := checkBuildVariants(project)

			So(validationResults.String(), ShouldContainSubstring, "WARNING: this is a warning")
		})

		Convey("if two variants have the same display name, a warning should be returned, but no errors", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux1", DisplayName: "foo"},
					{Name: "linux", DisplayName: "foo"},
				},
			}

			validationResults := checkBuildVariants(project)
			numErrors := len(validationResults.AtLevel(Error))
			numWarnings := len(validationResults.AtLevel(Warning))

			So(numWarnings, ShouldEqual, 3)
			So(numErrors, ShouldEqual, 0)
			So(len(validationResults), ShouldEqual, 3)
		})

		Convey("if several buildvariants have duplicate entries, all errors "+
			"should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux", DisplayName: "foo0"},
					{Name: "linux", DisplayName: "foo1"},
					{Name: "windows", DisplayName: "foo2"},
					{Name: "windows", DisplayName: "foo3"},
				},
			}
			So(validateBVNames(project), ShouldNotResemble, ValidationErrors{})
			So(len(validateBVNames(project)), ShouldEqual, 2)
		})

		Convey("if no buildvariants have duplicate entries, no error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "linux", DisplayName: "foo0"},
					{Name: "windows", DisplayName: "foo1"},
				},
			}
			So(validateBVNames(project), ShouldResemble, ValidationErrors{})
		})

		Convey("if a buildvariant name contains unauthorized characters, an error should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{Name: "|linux", DisplayName: "foo0"},
					{Name: "linux|", DisplayName: "foo1"},
					{Name: "wind|ows", DisplayName: "foo2"},
					{Name: "windows", DisplayName: "foo3"},
				},
			}
			So(validateBVNames(project), ShouldNotResemble, ValidationErrors{})
			So(len(validateBVNames(project)), ShouldEqual, 3)
		})
		Convey("A warning should be returned when a buildvariant name", func() {
			Convey("Contains commas", func() {
				project := &model.Project{
					BuildVariants: []model.BuildVariant{
						{Name: "variant,", DisplayName: "display_name"},
					},
				}
				buildVariant := project.BuildVariants[0]
				So(len(checkBVNames(&buildVariant)), ShouldEqual, 1)
			})
			Convey("Is the same as the all-dependencies syntax", func() {
				project := &model.Project{
					BuildVariants: []model.BuildVariant{
						{Name: model.AllVariants, DisplayName: "display_name"},
					},
				}
				buildVariant := project.BuildVariants[0]
				So(len(checkBVNames(&buildVariant)), ShouldEqual, 1)
			})
			Convey("Is 'all'", func() {
				project := &model.Project{
					BuildVariants: []model.BuildVariant{{Name: "all", DisplayName: "display_name"}},
				}
				buildVariant := project.BuildVariants[0]
				So(len(checkBVNames(&buildVariant)), ShouldEqual, 1)
			})
		})
	})
}

func TestValidateBVTaskNames(t *testing.T) {
	Convey("When validating a project's build variant's task names", t, func() {
		Convey("if any task has a duplicate entry, an error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux"},
							{Name: "compile", Variant: "linux"},
						},
					},
				},
			}
			So(validateBVTaskNames(project), ShouldNotResemble, ValidationErrors{})
			So(len(validateBVTaskNames(project)), ShouldEqual, 1)
		})

		Convey("if several task have duplicate entries, all errors should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux"},
							{Name: "compile", Variant: "linux"},
							{Name: "test", Variant: "linux"},
							{Name: "test", Variant: "linux"},
						},
					},
				},
			}
			So(validateBVTaskNames(project), ShouldNotResemble, ValidationErrors{})
			So(len(validateBVTaskNames(project)), ShouldEqual, 2)
		})

		Convey("if no tasks have duplicate entries, no error should be"+
			" returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux"},
							{Name: "test", Variant: "linux"},
						},
					},
				},
			}
			So(validateBVTaskNames(project), ShouldResemble, ValidationErrors{})
		})
	})
}

func TestValidateBVBatchTimes(t *testing.T) {
	batchtime := 126
	p := &model.Project{
		BuildVariants: []model.BuildVariant{
			{
				Name:          "linux",
				BatchTime:     &batchtime,
				CronBatchTime: "@notadescriptor",
			},
		},
	}
	// can't set cron and batchtime for build variants
	assert.Len(t, validateBVBatchTimes(p), 2)

	p.BuildVariants[0].BatchTime = nil
	p.BuildVariants[0].CronBatchTime = "@daily"
	assert.Empty(t, validateBVBatchTimes(p))

	// can have task and variant batchtime set
	p.BuildVariants[0].Tasks = []model.BuildVariantTaskUnit{
		{Name: "t1", Variant: p.BuildVariants[0].Name, BatchTime: &batchtime},
		{Name: "t2", Variant: p.BuildVariants[0].Name},
	}
	assert.Len(t, validateBVBatchTimes(p), 0)

	// can't set cron and batchtime for tasks
	p.BuildVariants[0].Tasks[0].CronBatchTime = "@daily"
	assert.Len(t, validateBVBatchTimes(p), 1)

	p.BuildVariants[0].Tasks[0].BatchTime = nil
	assert.Len(t, validateBVBatchTimes(p), 0)

	// warning if activated to true with batchtime
	p.BuildVariants[0].Activate = utility.TruePtr()
	bv := p.BuildVariants[0]
	assert.Len(t, checkBVBatchTimes(&bv), 1)

}

func TestCheckBVsContainTasks(t *testing.T) {
	Convey("When validating a project's build variants", t, func() {
		Convey("if any build variant contains no tasks an error should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux"},
						},
					},
					{
						Name:  "windows",
						Tasks: []model.BuildVariantTaskUnit{},
					},
				},
			}
			So(len(checkBuildVariants(project)), ShouldEqual, 2)
		})

		Convey("if all build variants contain tasks no errors should be returned", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux"},
						},
					},
					{
						Name: "windows",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "windows"},
						},
					},
				},
			}
			So(len(checkBuildVariants(project)), ShouldEqual, 1)
		})
	})
}

func TestValidateAllDependenciesSpec(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("if a task references all dependencies, no other dependency "+
			"should be specified. If one is, an error should be returned",
			func() {
				project := &model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "compile",
							DependsOn: []model.TaskUnitDependency{
								{Name: model.AllDependencies},
								{Name: "testOne"},
							},
						},
					},
				}
				So(validateAllDependenciesSpec(project), ShouldNotResemble,
					ValidationErrors{})
				So(len(validateAllDependenciesSpec(project)), ShouldEqual, 1)
			})
		Convey("if a task references only all dependencies, no error should "+
			"be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						DependsOn: []model.TaskUnitDependency{
							{Name: model.AllDependencies},
						},
					},
				},
			}
			So(validateAllDependenciesSpec(project), ShouldResemble, ValidationErrors{})
		})
		Convey("if a task references any other dependencies, no error should "+
			"be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						DependsOn: []model.TaskUnitDependency{
							{Name: "hello"},
						},
					},
				},
			}
			So(validateAllDependenciesSpec(project), ShouldResemble, ValidationErrors{})
		})
		Convey("if a task references all dependencies on multiple variants, no error should "+
			" be returned", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "coverage",
						DependsOn: []model.TaskUnitDependency{
							{
								Name:    "*",
								Variant: "ubuntu1604",
							},
							{
								Name:    "*",
								Variant: "coverage",
							},
						},
					},
				},
			}
			So(validateAllDependenciesSpec(project), ShouldResemble, ValidationErrors{})
		})
	})
}

func TestValidateProjectTaskNames(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure any duplicate task names throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{Name: "compile"},
				},
			}
			So(validateProjectTaskNames(project), ShouldNotResemble, ValidationErrors{})
			So(len(validateProjectTaskNames(project)), ShouldEqual, 1)
		})
		Convey("ensure unique task names do not throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
			}
			So(validateProjectTaskNames(project), ShouldResemble, ValidationErrors{})
		})
	})
}

func TestValidateProjectTaskIdsAndTags(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure bad task tags throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile", Tags: []string{"a", "!b", "ccc ccc", "d", ".e", "f\tf"}},
				},
			}
			So(validateProjectTaskIdsAndTags(project), ShouldNotResemble, ValidationErrors{})
			So(len(validateProjectTaskIdsAndTags(project)), ShouldEqual, 4)
		})
		Convey("ensure bad task names throw an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
					{Name: "!compile"},
					{Name: ".compile"},
					{Name: "Fun!"},
				},
			}
			So(validateProjectTaskIdsAndTags(project), ShouldNotResemble, ValidationErrors{})
			So(len(validateProjectTaskIdsAndTags(project)), ShouldEqual, 2)
		})
	})
}

func TestValidatePlugins(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(model.ProjectRefCollection),
		"Error clearing collection")
	projectRef := &model.ProjectRef{
		Enabled: true,
		Id:      "p1",
	}
	assert.Nil(projectRef.Insert())
	Convey("When validating a project", t, func() {
		Convey("ensure bad plugin configs throw an error", func() {
			So(validateProjectConfigPlugins(&model.ProjectConfig{}), ShouldResemble, ValidationErrors{})
			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			}}}), ShouldResemble, ValidationErrors{})

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			}}}), ShouldResemble, ValidationErrors{})

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			}}}), ShouldResemble, ValidationErrors{})

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject: "BFG",
			}}}), ShouldNotBeNil)

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketSearchProjects: []string{"BF", "BFG"},
			}}}), ShouldNotBeNil)

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionUsername:    "user",
				BFSuggestionPassword:    "pass",
				BFSuggestionTimeoutSecs: 10,
			}}}), ShouldResemble, ValidationErrors{})

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			}}}), ShouldResemble, ValidationErrors{})

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
				BFSuggestionUsername: "user",
				BFSuggestionPassword: "pass",
			}}}), ShouldNotBeNil)

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionTimeoutSecs: 10,
			}}}), ShouldNotBeNil)

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			}}}), ShouldNotBeNil)

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionPassword:    "pass",
				BFSuggestionTimeoutSecs: 10,
			}}}), ShouldNotBeNil)
			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 0,
			}}}), ShouldNotBeNil)

			So(validateProjectConfigPlugins(&model.ProjectConfig{Id: "", ProjectConfigFields: model.ProjectConfigFields{BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: -1,
			}}}), ShouldNotBeNil)
		})
	})
}

func TestValidateAliasCoverage(t *testing.T) {
	for testName, testCase := range map[string]func(*testing.T, *model.Project){
		"MatchesNothing": func(t *testing.T, p *model.Project) {
			alias1 := model.ProjectAlias{
				ID:          mgobson.NewObjectId(),
				Alias:       evergreen.CommitQueueAlias,
				VariantTags: []string{"notTheVariantTag"},
				TaskTags:    []string{"taskTag1", "taskTag2"},
				Source:      model.AliasSourceConfig,
			}
			alias2 := model.ProjectAlias{
				ID:      mgobson.NewObjectId(),
				Alias:   evergreen.CommitQueueAlias,
				Variant: "nonsense",
				Task:    ".*",
				Source:  model.AliasSourceConfig,
			}
			aliasMap := map[string]model.ProjectAlias{
				"alias1": alias1,
				"alias2": alias2,
			}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliasMap)
			assert.NoError(t, err)
			assert.Len(t, needsVariants, 2)
			assert.Len(t, needsTasks, 2)
			for _, matches := range needsVariants {
				assert.True(t, matches)
			}
			// Doesn't matter that the tasks match since the variants don't match
			for _, matches := range needsTasks {
				assert.True(t, matches)
			}
			errs := validateAliasCoverage(p, model.ProjectAliases{alias1, alias2})
			require.Len(t, errs, 2)
			assert.Contains(t, errs[0].Message, "Commit queue alias")
			assert.Contains(t, errs[0].Message, "(from the yaml)")
			assert.Contains(t, errs[0].Message, "has no matching variants")
			assert.Contains(t, errs[1].Message, "Commit queue alias")
			assert.Contains(t, errs[1].Message, "(from the yaml)")
			assert.Contains(t, errs[1].Message, "has no matching variants")
			assert.NotContains(t, errs[0].Message, "tasks")
			assert.NotContains(t, errs[1].Message, "tasks")
			assert.Equal(t, errs[0].Level, Warning)
			assert.Equal(t, errs[1].Level, Warning)
		},
		"MatchesAll": func(t *testing.T, p *model.Project) {
			alias1 := model.ProjectAlias{
				ID:          mgobson.NewObjectId(),
				Alias:       evergreen.CommitQueueAlias,
				VariantTags: []string{"variantTag"},
				TaskTags:    []string{"taskTag1"},
			}
			alias2 := model.ProjectAlias{
				ID:      mgobson.NewObjectId(),
				Alias:   evergreen.CommitQueueAlias,
				Variant: "bvWith.*",
				Task:    ".*",
			}
			aliasMap := map[string]model.ProjectAlias{
				"alias1": alias1,
				"alias2": alias2,
			}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliasMap)
			assert.NoError(t, err)
			assert.Len(t, needsVariants, 2)
			assert.Len(t, needsTasks, 2)
			for _, matches := range needsVariants {
				assert.False(t, matches)
			}
			for _, matches := range needsTasks {
				assert.False(t, matches)
			}
			errs := validateAliasCoverage(p, model.ProjectAliases{alias1, alias2})
			assert.Len(t, errs, 0)
		},
		"MatchesVariantTag": func(t *testing.T, p *model.Project) {
			alias1 := model.ProjectAlias{
				ID:          mgobson.NewObjectId(),
				Alias:       evergreen.CommitQueueAlias,
				VariantTags: []string{"variantTag"},
				Source:      model.AliasSourceProject,
			}
			alias2 := model.ProjectAlias{
				ID:      mgobson.NewObjectId(),
				Alias:   evergreen.CommitQueueAlias,
				Variant: "badRegex",
				Source:  model.AliasSourceProject,
			}
			aliasMap := map[string]model.ProjectAlias{
				"alias1": alias1,
				"alias2": alias2,
			}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliasMap)
			assert.NoError(t, err)
			assert.Len(t, needsVariants, 2)
			assert.Len(t, needsTasks, 2)
			assert.False(t, needsVariants["alias1"])
			assert.True(t, needsVariants["alias2"])
			for _, matches := range needsTasks {
				assert.True(t, matches)
			}

			errs := validateAliasCoverage(p, model.ProjectAliases{alias1, alias2})
			require.Len(t, errs, 2)
			assert.Contains(t, errs[0].Message, "Commit queue alias")
			assert.Contains(t, errs[0].Message, "(from the project page)")
			assert.Contains(t, errs[0].Message, "has no matching variants")
			assert.NotContains(t, errs[0].Message, "matching task regexp")
			assert.Contains(t, errs[1].Message, "Commit queue alias")
			assert.Contains(t, errs[1].Message, "(from the project page)")
			assert.Contains(t, errs[1].Message, "has no matching tasks")
			assert.Contains(t, errs[1].Message, "variant tags")
			assert.Contains(t, errs[1].Message, "matching task regexp")
			assert.Equal(t, errs[0].Level, Warning)
			assert.Equal(t, errs[1].Level, Warning)
		},
		"NegatedTag": func(t *testing.T, p *model.Project) {
			negatedAlias := model.ProjectAlias{
				ID:          mgobson.NewObjectId(),
				Alias:       evergreen.CommitQueueAlias,
				VariantTags: []string{"!variantTag"},
				TaskTags:    []string{"!newTaskTag"},
			}
			aliasMap := map[string]model.ProjectAlias{
				"negatedAlias": negatedAlias,
			}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliasMap)
			assert.NoError(t, err)
			assert.Len(t, needsVariants, 1)
			assert.Len(t, needsTasks, 1)
			assert.False(t, needsVariants["negatedAlias"]) // Matches the second build variant
			assert.False(t, needsTasks["negatedAlias"])

			for i := 1; i < len(p.BuildVariants); i++ {
				p.BuildVariants[i].Tags = []string{"variantTag"}
			}
			needsVariants, needsTasks, err = getAliasCoverage(p, aliasMap)
			assert.NoError(t, err)
			assert.Len(t, needsVariants, 1)
			assert.Len(t, needsTasks, 1)
			assert.True(t, needsVariants["negatedAlias"]) // Doesn't match any build variant
			assert.True(t, needsTasks["negatedAlias"])    // Because the variants don't match

			p.BuildVariants[1].Tags = nil
			p.Tasks[1].Tags = []string{"newTaskTag"}
			needsVariants, needsTasks, err = getAliasCoverage(p, aliasMap)
			assert.NoError(t, err)
			assert.Len(t, needsVariants, 1)
			assert.Len(t, needsTasks, 1)
			assert.False(t, needsVariants["negatedAlias"]) // Matches the second build variant again
			assert.True(t, needsTasks["negatedAlias"])     // Second build variant task doesn't match
		},
		"MatchesTaskInTaskGroupWithTaskRegexp": func(t *testing.T, p *model.Project) {
			a := model.ProjectAlias{
				ID:      mgobson.NewObjectId(),
				Alias:   "alias",
				Variant: "bvWithTaskGroup",
				Task:    "taskWithoutTag",
			}
			aliases := map[string]model.ProjectAlias{a.Alias: a}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliases)
			require.NoError(t, err)
			assert.Len(t, needsVariants, len(aliases))
			assert.Len(t, needsTasks, len(aliases))
			for _, noMatch := range needsVariants {
				assert.False(t, noMatch)
			}
			for _, noMatch := range needsTasks {
				assert.False(t, noMatch)
			}
			errs := validateAliasCoverage(p, model.ProjectAliases{a})
			assert.Len(t, errs, 0)
		},
		"MatchesTaskInTaskGroupWithTaskTag": func(t *testing.T, p *model.Project) {
			a := model.ProjectAlias{
				ID:       mgobson.NewObjectId(),
				Alias:    "alias",
				Variant:  "bvWithTaskGroup",
				TaskTags: []string{"taskTag1"},
			}
			aliases := map[string]model.ProjectAlias{a.Alias: a}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliases)
			require.NoError(t, err)
			assert.Len(t, needsVariants, len(aliases))
			assert.Len(t, needsTasks, len(aliases))
			for _, noMatch := range needsVariants {
				assert.False(t, noMatch)
			}
			for _, noMatch := range needsTasks {
				assert.False(t, noMatch)
			}
			errs := validateAliasCoverage(p, model.ProjectAliases{a})
			assert.Len(t, errs, 0)
		},
		"MatchesTaskWithTaskTagHavingMultipleCriteria": func(t *testing.T, p *model.Project) {
			a := model.ProjectAlias{
				ID:       mgobson.NewObjectId(),
				Alias:    "alias",
				Variant:  "bvWithTag",
				TaskTags: []string{"taskTag1 taskTag2"},
			}
			aliases := map[string]model.ProjectAlias{a.Alias: a}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliases)
			require.NoError(t, err)
			assert.Len(t, needsVariants, len(aliases))
			assert.Len(t, needsTasks, len(aliases))
			for _, noMatch := range needsVariants {
				assert.False(t, noMatch)
			}
			for _, noMatch := range needsTasks {
				assert.False(t, noMatch)
			}
			errs := validateAliasCoverage(p, model.ProjectAliases{a})
			assert.Len(t, errs, 0)
		},
		"DoesNotMatchTaskWithTaskTagHavingMultipleCriteria": func(t *testing.T, p *model.Project) {
			a := model.ProjectAlias{
				ID:       mgobson.NewObjectId(),
				Alias:    "alias",
				Variant:  "bvWithTag",
				TaskTags: []string{"taskTag1 taskTag2 nonexistent"},
				Source:   model.AliasSourceRepo,
			}
			aliases := map[string]model.ProjectAlias{a.Alias: a}
			needsVariants, needsTasks, err := getAliasCoverage(p, aliases)
			require.NoError(t, err)
			assert.Len(t, needsVariants, len(aliases))
			assert.Len(t, needsTasks, len(aliases))
			for _, noMatch := range needsVariants {
				assert.False(t, noMatch)
			}
			for _, noMatch := range needsTasks {
				assert.True(t, noMatch)
			}
			errs := validateAliasCoverage(p, model.ProjectAliases{a})
			assert.Len(t, errs, 1)
			assert.Contains(t, errs[0].Message, "(from the repo page)")
			assert.Contains(t, errs[0].Message, "Patch alias 'alias'")
		},
	} {
		t.Run(testName, func(t *testing.T) {
			p := &model.Project{
				BuildVariants: model.BuildVariants{
					{
						Name: "bvWithTag",
						Tags: []string{"variantTag"},
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "taskWithTag",
								Variant: "bvWithTag",
							},
						},
					},
					{
						Name: "bvWithoutTag",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "taskWithoutTag",
								Variant: "bvWithoutTag",
							},
						},
					},
					{
						Name: "bvWithTaskGroup",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "taskGroup",
								Variant: "bvWithTaskGroup",
								IsGroup: true,
							},
						},
					},
				},
				Tasks: []model.ProjectTask{
					{
						Name: "taskWithTag",
						Tags: []string{"taskTag1", "taskTag2"},
					},
					{
						Name: "taskWithoutTag",
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "taskGroup",
						Tasks: []string{"taskWithTag", "taskWithoutTag"},
					},
				},
			}
			testCase(t, p)
		})
	}
}

func TestValidateProjectAliases(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure misconfigured aliases throw an error", func() {
			projectConfig := &model.ProjectConfig{
				Id: "project-1",
				ProjectConfigFields: model.ProjectConfigFields{
					PatchAliases: []model.ProjectAlias{
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Alias:     "",
							Variant:   "v1",
							Task:      "^test",
						},
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Alias:     "alias-1",
							Task:      "^test",
						},
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Alias:     "alias-1",
							Variant:   "v1",
						},
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Alias:     "alias-1",
							Variant:   "[0-9]++",
							Task:      "^test",
						},
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Alias:     "alias-1",
							Variant:   "v1",
							Task:      "[0-9]++",
						},
					},
					CommitQueueAliases: []model.ProjectAlias{
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Variant:   "v1",
							Task:      "^test",
						},
					},
					GitHubChecksAliases: []model.ProjectAlias{
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Variant:   "v1",
							Task:      "^test",
						},
					},
					GitTagAliases: []model.ProjectAlias{
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Variant:   "v1",
							Task:      "^test",
						},
						{
							ID:        mgobson.NewObjectId(),
							ProjectID: "project-1",
							Variant:   "v1",
							Task:      "^test",
							GitTag:    "[0-9]++",
						},
						{
							ID:         mgobson.NewObjectId(),
							ProjectID:  "project-1",
							Variant:    "v1",
							Task:       "^test",
							RemotePath: "remote/path",
							GitTag:     "^test",
						},
					},
				},
			}
			validationErrs := validateProjectConfigAliases(projectConfig)
			So(validationErrs, ShouldNotResemble, ValidationErrors{})
			So(len(validationErrs), ShouldEqual, 8)
			So(validationErrs[0].Message, ShouldContainSubstring, "can't be empty string")
			So(validationErrs[1].Message, ShouldContainSubstring, "must specify exactly one of variant regex")
			So(validationErrs[2].Message, ShouldContainSubstring, "must specify exactly one of task regex")
			So(validationErrs[3].Message, ShouldContainSubstring, "variant regex #4 is invalid")
			So(validationErrs[4].Message, ShouldContainSubstring, "task regex #5 is invalid")
			So(validationErrs[5].Message, ShouldContainSubstring, "must define valid git tag regex")
			So(validationErrs[6].Message, ShouldContainSubstring, "git tag regex #2 is invalid")
			So(validationErrs[7].Message, ShouldContainSubstring, "cannot define remote path")
		})
	})
}

func TestCheckTaskCommands(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("ensure tasks that do not have at least one command throw "+
			"an error", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
			}
			errs := checkTasks(project)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 2)
			assert.Contains(t, errs.String(), "task 'compile' does not "+
				"contain any commands")
		})
		Convey("ensure tasks that have at least one command do not throw any errors",
			func() {
				project := &model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "compile",
							Commands: []model.PluginCommandConf{
								{
									Command: "gotest.parse_files",
									Params: map[string]interface{}{
										"files": []interface{}{"test"},
									},
								},
							},
						},
					},
				}
				So(validateProjectTaskNames(project), ShouldResemble, ValidationErrors{})
			})
		Convey("ensure that plugin commands have setup type",
			func() {
				project := &model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "compile",
							Commands: []model.PluginCommandConf{
								{
									Command: "gotest.parse_files",
									Type:    "setup",
									Params: map[string]interface{}{
										"files": []interface{}{"test"},
									},
								},
							},
						},
					},
				}
				So(validateProjectTaskNames(project), ShouldResemble, ValidationErrors{})
			})
	})
}

func TestEnsureReferentialIntegrity(t *testing.T) {
	Convey("When validating a project", t, func() {
		distroIds := []string{"rhel55"}
		distroAliases := []string{"rhel55-alias"}
		distroWarnings := map[string]string{
			"rhel55":       "55 is not the best number",
			"rhel55-alias": "and this is not the best alias",
		}
		Convey("an error should be thrown if a referenced task for a "+
			"buildvariant does not exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "test", Variant: "linux"},
						},
					},
				},
			}
			errs := ensureReferentialIntegrity(project, nil, distroIds, distroAliases, nil)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced task for a "+
			"buildvariant does exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux", RunOn: []string{"rhel55"}},
						},
					},
				},
			}
			So(ensureReferentialIntegrity(project, nil, distroIds, distroAliases, nil), ShouldResemble,
				ValidationErrors{})
		})
		Convey("an error should be thrown if a task references a distro has a warning", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "linux",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux", RunOn: []string{"rhel55"}},
						},
					},
				},
			}
			errs := ensureReferentialIntegrity(project, nil, distroIds, distroAliases, distroWarnings)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs.AtLevel(Notice)), ShouldEqual, 1)
			So(errs[0].Message, ShouldContainSubstring, "distro 'rhel55' with the following admin-defined warning(s): 55 is not the best number")
		})
		Convey("an error should be thrown if a variant references a distro has a warning", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "compile"},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name:  "linux",
						RunOn: []string{"rhel55-alias"},
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "compile", Variant: "linux"},
						},
					},
				},
			}
			errs := ensureReferentialIntegrity(project, nil, distroIds, distroAliases, distroWarnings)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs.AtLevel(Notice)), ShouldEqual, 1)
			So(errs[0].Message, ShouldContainSubstring, "distro 'rhel55-alias' with the following admin-defined warning: and this is not the best alias")
		})
		Convey("an error should be thrown if a referenced distro for a "+
			"buildvariant does not exist", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name:  "enterprise",
						RunOn: []string{"hello"},
					},
				},
			}
			errs := ensureReferentialIntegrity(project, nil, distroIds, distroAliases, nil)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})

		Convey("an error should be thrown if a referenced distro for a "+
			"buildvariant has the same name as an existing container", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name:  "enterprise",
						RunOn: []string{"rhel55"},
					},
				},
			}
			containerNameMap := map[string]bool{
				"rhel55": true,
			}
			errs := ensureReferentialIntegrity(project, containerNameMap, distroIds, distroAliases, nil)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 2)
			So(errs[0].Message, ShouldContainSubstring, "buildvariant 'enterprise' references a container name overlapping with an existing distro 'rhel55'")
			So(errs[1].Message, ShouldContainSubstring, "run_on cannot contain a mixture of containers and distros")
		})

		Convey("an error should be thrown if a buildvariant references a mix of distros and containers to run on", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name:  "enterprise",
						RunOn: []string{"rhel55", "c1"},
					},
				},
			}
			containerNameMap := map[string]bool{
				"c1": true,
			}
			errs := ensureReferentialIntegrity(project, containerNameMap, distroIds, distroAliases, nil)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
			So(errs[0].Message, ShouldContainSubstring, "run_on cannot contain a mixture of containers and distros")
		})

		Convey("no error should be thrown if a referenced distro ID for a "+
			"buildvariant does exist", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name:  "enterprise",
						RunOn: distroIds,
					},
				},
			}
			So(ensureReferentialIntegrity(project, nil, distroIds, distroAliases, nil), ShouldResemble, ValidationErrors{})
		})

		Convey("no error should be thrown if a referenced distro alias for a"+
			"buildvariant does exist", func() {
			project := &model.Project{
				BuildVariants: []model.BuildVariant{
					{
						Name:  "enterprise",
						RunOn: distroAliases,
					},
				},
			}
			So(ensureReferentialIntegrity(project, nil, distroIds, distroAliases, nil), ShouldResemble, ValidationErrors{})
		})
	})
}

func TestValidateProjectConfigContainers(t *testing.T) {
	t.Run("SucceedsWithValidContainers", func(t *testing.T) {
		pc := model.ProjectConfig{
			ProjectConfigFields: model.ProjectConfigFields{
				ContainerSizeDefinitions: []model.ContainerResources{
					{
						Name:     "small",
						CPU:      128,
						MemoryMB: 128,
					},
					{
						Name:     "large",
						CPU:      2048,
						MemoryMB: 2048,
					},
				},
			},
		}
		errs := validateProjectConfigContainers(&pc)
		assert.Empty(t, errs)
	})
	t.Run("FailsWithInvalidContainerResources", func(t *testing.T) {
		pc := model.ProjectConfig{
			ProjectConfigFields: model.ProjectConfigFields{
				ContainerSizeDefinitions: []model.ContainerResources{
					{
						Name:     "invalid",
						CPU:      -10,
						MemoryMB: -5,
					},
				},
			},
		}
		errs := validateProjectConfigContainers(&pc)
		assert.NotEmpty(t, errs)
	})
	t.Run("FailsWithUnnamedContainerSize", func(t *testing.T) {
		pc := model.ProjectConfig{
			ProjectConfigFields: model.ProjectConfigFields{
				ContainerSizeDefinitions: []model.ContainerResources{
					{
						Name:     "",
						CPU:      128,
						MemoryMB: 128,
					},
				},
			},
		}
		errs := validateProjectConfigContainers(&pc)
		assert.NotEmpty(t, errs)
	})
	t.Run("FailsWithContainerSizeExceedingGlobalLimits", func(t *testing.T) {
		env := evergreen.GetEnvironment()
		originalECSConf := env.Settings().Providers.AWS.Pod.ECS
		defer func() {
			env.Settings().Providers.AWS.Pod.ECS = originalECSConf
		}()
		env.Settings().Providers.AWS.Pod.ECS = evergreen.ECSConfig{
			MaxCPU:      1024,
			MaxMemoryMB: 2048,
		}

		t.Run("CPU", func(t *testing.T) {
			pc := model.ProjectConfig{
				ProjectConfigFields: model.ProjectConfigFields{
					ContainerSizeDefinitions: []model.ContainerResources{
						{
							Name:     "xlarge",
							CPU:      100000000,
							MemoryMB: 100,
						},
					},
				},
			}
			errs := validateProjectConfigContainers(&pc)
			assert.NotEmpty(t, errs)
		})
		t.Run("Memory", func(t *testing.T) {
			pc := model.ProjectConfig{
				ProjectConfigFields: model.ProjectConfigFields{
					ContainerSizeDefinitions: []model.ContainerResources{
						{
							Name:     "xlarge",
							CPU:      100,
							MemoryMB: 100000000,
						},
					},
				},
			}
			errs := validateProjectConfigContainers(&pc)
			assert.NotEmpty(t, errs)
		})
	})
}

func TestValidatePluginCommands(t *testing.T) {
	Convey("When validating a project", t, func() {
		Convey("an error should be thrown if a referenced plugin for a task does not exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "a.b",
								Params:   map[string]interface{}{},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("an error should be thrown if a referenced function command is invalid (invalid params)", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "gotest.parse_files",
							Params: map[string]interface{}{
								"blah": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("an error should be thrown if a shell.exec command has misspelled params", func() {
			exampleYml := `
tasks:
- name: example_task
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    parms:
      script: echo test
`
			proj := model.Project{}
			ctx := context.Background()
			pp, err := model.LoadProjectInto(ctx, []byte(exampleYml), nil, "example_project", &proj)
			So(pp, ShouldNotBeNil)
			So(proj, ShouldNotBeNil)
			So(err, ShouldBeNil)
			validationErrs := validatePluginCommands(&proj)
			So(validationErrs, ShouldNotResemble, ValidationErrors{})
			So(len(validationErrs.AtLevel(Error)), ShouldEqual, 1)
			So(validationErrs.AtLevel(Error)[0].Message, ShouldContainSubstring, "params cannot be nil")
		})
		Convey("an error should return if a shell.exec command is missing a script", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "shell.exec",
							Type:    "system",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			validationErrs := validatePluginCommands(project)
			So(validationErrs, ShouldNotResemble, ValidationErrors{})
			So(len(validationErrs.AtLevel(Error)), ShouldEqual, 1)
			So(validationErrs.AtLevel(Error)[0].Message, ShouldContainSubstring, "must specify a script")
		})
		Convey("an error should not be thrown if a shell.exec command is defined with a script", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "shell.exec",
							Type:    "system",
							Params: map[string]interface{}{
								"script": "echo hi",
							},
						},
					},
				},
			}
			validationErrs := validatePluginCommands(project)
			So(validationErrs, ShouldResemble, ValidationErrors{})
			So(len(validationErrs.AtLevel(Error)), ShouldEqual, 0)
		})
		Convey("an error should be thrown if a shell.exec command is missing params", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "shell.exec",
							Type:    "system",
						},
					},
				},
			}
			validationErrs := validatePluginCommands(project)
			So(validationErrs, ShouldNotResemble, ValidationErrors{})
			So(len(validationErrs.AtLevel(Error)), ShouldEqual, 1)
			So(validationErrs.AtLevel(Error)[0].Message, ShouldContainSubstring, "params cannot be nil")
		})
		Convey("an error should be thrown if both a function and a plugin command are referenced", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "funcOne",
								Command:  "gotest.parse_files",
								Params: map[string]interface{}{
									"files": []interface{}{"test"},
								},
							},
						},
					},
				},
			}
			errs := validatePluginCommands(project)
			So(errs, ShouldNotResemble, ValidationErrors{})
			So(len(errs), ShouldEqual, 1)
		})
		Convey("an error should be thrown if a function plugin command doesn't have commands", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Params: map[string]interface{}{
								"blah": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a function plugin command is valid", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"funcOne": {
						SingleCommand: &model.PluginCommandConf{
							Command: "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
		})
		Convey("an error should be thrown if a function 'a' references "+
			"any another function", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"a": {
						SingleCommand: &model.PluginCommandConf{
							Function: "b",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
					"b": {
						SingleCommand: &model.PluginCommandConf{
							Command: "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 2)
		})
		Convey("errors should be thrown if a function 'a' references "+
			"another function, 'b', which that does not exist", func() {
			project := &model.Project{
				Functions: map[string]*model.YAMLCommandSet{
					"a": {
						SingleCommand: &model.PluginCommandConf{
							Function: "b",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 3)
		})

		Convey("an error should be thrown if a referenced pre plugin command is invalid", func() {
			project := &model.Project{
				Pre: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Command: "gotest.parse_files",
							Params:  map[string]interface{}{},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced pre plugin command is valid", func() {
			project := &model.Project{
				Pre: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
		})
		Convey("an error should be thrown if a referenced post plugin command is invalid", func() {
			project := &model.Project{
				Post: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params:   map[string]interface{}{},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced post plugin command is valid", func() {
			project := &model.Project{
				Post: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
		})
		Convey("an error should be thrown if a referenced timeout plugin command is invalid", func() {
			project := &model.Project{
				Timeout: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params:   map[string]interface{}{},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced timeout plugin command is valid", func() {
			project := &model.Project{
				Timeout: &model.YAMLCommandSet{
					MultiCommand: []model.PluginCommandConf{
						{
							Function: "",
							Command:  "gotest.parse_files",
							Params: map[string]interface{}{
								"files": []interface{}{"test"},
							},
						},
					},
				},
			}

			So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
		})
		Convey("no error should be thrown if a referenced plugin for a task does exist", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "archive.targz_pack",
								Params: map[string]interface{}{
									"target":     "tgz",
									"source_dir": "src",
									"include":    []string{":"},
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
		})
		Convey("no error should be thrown if a referenced plugin that exists contains unneeded parameters", func() {
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "archive.targz_pack",
								Params: map[string]interface{}{
									"target":     "tgz",
									"source_dir": "src",
									"include":    []string{":"},
									"extraneous": "G",
								},
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
		})
		Convey("an error should be thrown if a referenced plugin contains invalid parameters", func() {
			params := map[string]interface{}{
				"aws_key":    "key",
				"aws_secret": "sec",
				"s3_copy_files": []interface{}{
					map[string]interface{}{
						"source": map[string]interface{}{
							"bucket": "long3nough",
							"path":   "fghij",
						},
						"destination": map[string]interface{}{
							"bucket": "..long-but-invalid",
							"path":   "fghij",
						},
					},
				},
			}
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "s3Copy.copy",
								Params:   params,
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldNotResemble, ValidationErrors{})
			So(len(validatePluginCommands(project)), ShouldEqual, 1)
		})
		Convey("no error should be thrown if a referenced plugin that "+
			"exists contains params that appear invalid but are in expansions",
			func() {
				params := map[string]interface{}{
					"aws_key":    "key",
					"aws_secret": "sec",
					"s3_copy_files": []interface{}{
						map[string]interface{}{
							"source": map[string]interface{}{
								"bucket": "long3nough",
								"path":   "fghij",
							},
							"destination": map[string]interface{}{
								"bucket": "${..longButInvalid}",
								"path":   "fghij",
							},
						},
					},
				}
				project := &model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "compile",
							Commands: []model.PluginCommandConf{
								{
									Function: "",
									Command:  "s3Copy.copy",
									Params:   params,
								},
							},
						},
					},
				}
				So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
			})
		Convey("no error should be thrown if a referenced plugin contains all "+
			"the necessary and valid parameters", func() {
			params := map[string]interface{}{
				"aws_key":    "key",
				"aws_secret": "sec",
				"s3_copy_files": []interface{}{
					map[string]interface{}{
						"source": map[string]interface{}{
							"bucket": "abcde",
							"path":   "fghij",
						},
						"destination": map[string]interface{}{
							"bucket": "abcde",
							"path":   "fghij",
						},
					},
				},
			}
			project := &model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "compile",
						Commands: []model.PluginCommandConf{
							{
								Function: "",
								Command:  "s3Copy.copy",
								Params:   params,
							},
						},
					},
				},
			}
			So(validatePluginCommands(project), ShouldResemble, ValidationErrors{})
		})
	})
}

func TestCheckProjectWarnings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When validating a project's semantics", t, func() {
		Convey("if the project passes all of the validation funcs, no errors"+
			" should be returned", func() {
			distros := []distro.Distro{
				{Id: "test-distro-one"},
				{Id: "test-distro-two"},
			}

			for _, d := range distros {
				So(d.Insert(ctx), ShouldBeNil)
			}

			projectRef := &model.ProjectRef{
				Id: "project_test",
			}
			v := &model.Version{
				Id:         "my_version",
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
				Identifier: "project_test",
				Requester:  evergreen.RepotrackerVersionRequester,
			}
			pp := model.ParserProject{
				Id: "my_version",
			}

			require.NoError(t, pp.Insert())
			require.NoError(t, v.Insert(), "failed to insert test version: %v", v)
			_, project, _, err := model.FindLatestVersionWithValidProject(projectRef.Id, false)
			So(err, ShouldBeNil)
			So(CheckProjectWarnings(project), ShouldResemble, ValidationErrors{})
		})

		Reset(func() {
			So(db.ClearCollections(distro.Collection, model.ParserProjectCollection, model.VersionCollection), ShouldBeNil)
		})
	})
}

type validateProjectFieldSuite struct {
	suite.Suite
	project model.Project
}

func TestValidateProjectFieldSuite(t *testing.T) {
	suite.Run(t, new(validateProjectFieldSuite))
}

func (s *validateProjectFieldSuite) SetupTest() {
	s.project = model.Project{
		Identifier:  "identifier",
		DisplayName: "test",
	}
}

func (s *validateProjectFieldSuite) TestCommandTypes() {
	s.project.CommandType = "system"
	validationError := validateProjectFields(&s.project)
	s.Empty(validationError)

	s.project.CommandType = "test"
	validationError = validateProjectFields(&s.project)
	s.Empty(validationError)

	s.project.CommandType = "setup"
	validationError = validateProjectFields(&s.project)
	s.Empty(validationError)

	s.project.CommandType = ""
	validationError = validateProjectFields(&s.project)
	s.Empty(validationError)
}

func (s *validateProjectFieldSuite) TestFailOnInvalidCommandType() {
	s.project.CommandType = "random"
	validationError := validateProjectFields(&s.project)

	s.Len(validationError, 1)
	s.Contains(validationError[0].Message, "invalid command type: random",
		"Project 'CommandType' must be valid")
}

func TestValidateBVFields(t *testing.T) {
	Convey("When ensuring necessary buildvariant fields are set, ensure that", t, func() {
		Convey("an error is thrown if no build variants exist", func() {
			project := &model.Project{
				Identifier: "test",
			}
			So(validateBVFields(project),
				ShouldNotResemble, ValidationErrors{})
			So(len(validateBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("buildvariants with none of the necessary fields set throw errors", func() {
			project := &model.Project{
				Identifier:    "test",
				BuildVariants: []model.BuildVariant{{}},
			}
			So(validateBVFields(project),
				ShouldNotResemble, ValidationErrors{})
			So(len(validateBVFields(project)),
				ShouldEqual, 2)
		})
		Convey("an error is thrown if the buildvariant does not have a "+
			"name field set", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						RunOn: []string{"mongo"},
						Tasks: []model.BuildVariantTaskUnit{{Name: "db", Variant: "mongo"}},
					},
				},
			}
			So(validateBVFields(project),
				ShouldNotResemble, ValidationErrors{})
			So(len(validateBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("an error is thrown if the buildvariant does not have any tasks set", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name:  "postal",
						RunOn: []string{"service"},
					},
				},
			}
			So(validateBVFields(project),
				ShouldNotResemble, ValidationErrors{})
			So(len(validateBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("no error is thrown if the buildvariant has a run_on field set", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name:  "import",
						RunOn: []string{"export"},
						Tasks: []model.BuildVariantTaskUnit{{Name: "db", Variant: "import"}},
					},
				},
			}
			So(validateBVFields(project),
				ShouldResemble, ValidationErrors{})
		})
		Convey("an error should be thrown if the buildvariant has no "+
			"run_on field and at least one task has no distro field "+
			"specified", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name:  "import",
						Tasks: []model.BuildVariantTaskUnit{{Name: "db", Variant: "import"}},
					},
				},
			}
			So(validateBVFields(project),
				ShouldNotResemble, ValidationErrors{})
			So(len(validateBVFields(project)),
				ShouldEqual, 1)
		})
		Convey("no error should be thrown if the buildvariant does not "+
			"have a run_on field specified but the task definition has a "+
			"distro field specified", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name: "import",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "silhouettes",
								Variant: "import",
							},
						},
					},
				},
				Tasks: []model.ProjectTask{
					{
						Name: "silhouettes",
						RunOn: []string{
							"echoes",
						},
					},
				},
			}
			So(validateBVFields(project),
				ShouldResemble, ValidationErrors{})
		})
		Convey("no error should be thrown if the buildvariant does not "+
			"have a run_on field specified but all tasks within it have a "+
			"distro field specified", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name: "import",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "silhouettes",
								Variant: "import",
								RunOn: []string{
									"echoes",
								},
							},
						},
					},
				},
			}
			So(validateBVFields(project),
				ShouldResemble, ValidationErrors{})
		})
		Convey("no error should be thrown if the task group does not "+
			"have a run_on field specified but all tasks within it have a "+
			"distro field specified", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name: "import",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "group",
								Variant: "import",
								IsGroup: true,
								RunOn: []string{
									"echoes",
								},
							},
						},
					},
				},
				Tasks: []model.ProjectTask{
					{
						Name: "silhouettes",
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "group",
						Tasks: []string{"silhouettes"},
					},
				},
			}
			So(validateBVFields(project),
				ShouldResemble, ValidationErrors{})
		})
		Convey("no error should be thrown if the buildvariant does not "+
			"have a run_on field but all tasks within the specified task group has the "+
			"distro field specified", func() {
			project := &model.Project{
				Identifier: "projectId",
				BuildVariants: []model.BuildVariant{
					{
						Name: "import",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "group",
								Variant: "import",
								IsGroup: true,
							},
						},
					},
				},
				Tasks: []model.ProjectTask{
					{
						Name: "silhouettes",
						RunOn: []string{
							"echoes",
						},
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "group",
						Tasks: []string{"silhouettes"},
					},
				},
			}
			So(validateBVFields(project),
				ShouldResemble, ValidationErrors{})
		})
		Convey("blank distros should generate errors", func() {
			project := &model.Project{
				BuildVariants: model.BuildVariants{
					{
						Name:  "bv1",
						RunOn: []string{""},
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "t1",
								Variant: "bv1",
								RunOn:   []string{""}},
						},
					},
				},
			}
			So(validateBVFields(project),
				ShouldResemble, ValidationErrors{
					{Level: Error, Message: "buildvariant 'bv1' must either specify run_on field or have every task specify run_on"},
				})
		})
	})
}
func TestTaskValidation(t *testing.T) {
	assert.New(t)
	simpleYml := `
  tasks:
  - name: task0
  - name: this task is too long
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - name: task0
    - name: "this task is too long"
`
	var proj model.Project
	ctx := context.Background()
	_, err := model.LoadProjectInto(ctx, []byte(simpleYml), nil, "", &proj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spaces are not allowed")
}

func TestTaskGroupValidation(t *testing.T) {
	assert := assert.New(t)

	// check that yml with a task group with a duplicate task errors
	duplicateYml := `
  tasks:
  - name: example_task_1
  - name: example_task_2
  task_groups:
  - name: example_task_group
    tasks:
    - example_task_1
    - example_task_2
    - example_task_1
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - name: example_task_group
  `
	var proj model.Project
	ctx := context.Background()
	pp, err := model.LoadProjectInto(ctx, []byte(duplicateYml), nil, "", &proj)
	assert.NotNil(proj)
	assert.NotNil(pp)
	assert.NoError(err)
	validationErrs := validateTaskGroups(&proj)
	assert.Len(validationErrs, 1)
	assert.Contains(validationErrs[0].Message, "example_task_1 is listed in task group example_task_group 2 times")

	proj = model.Project{
		Tasks: []model.ProjectTask{
			{Name: "task1"},
		},
		TaskGroups: []model.TaskGroup{
			{
				Name:  "tg1",
				Tasks: []string{"task1"},
			},
			{
				Name:  "tg1",
				Tasks: []string{"task1"},
			},
		},
	}
	validationErrs = checkTaskGroups(&proj)
	require.Len(t, validationErrs, 1)
	assert.Contains(validationErrs[0].Message, "task group 'tg1' is defined multiple times; only the first will be used")

	// check that yml with a task group named the same as a task errors
	duplicateTaskYml := `
  tasks:
  - name: foo
  - name: example_task_2
  task_groups:
  - name: foo
    tasks:
    - example_task_2
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - name: foo
  `
	pp, err = model.LoadProjectInto(ctx, []byte(duplicateTaskYml), nil, "", &proj)
	assert.NotNil(proj)
	assert.NotNil(pp)
	assert.NoError(err)
	validationErrs = validateTaskGroups(&proj)
	assert.Len(validationErrs, 1)
	assert.Contains(validationErrs[0].Message, "foo is used as a name for both a task and task group")

	largeMaxHostYml := `
tasks:
- name: example_task_1
- name: example_task_2
- name: example_task_3
task_groups:
- name: example_task_group
  max_hosts: 4
  teardown_group:
  - command: attach.results
  tasks:
  - example_task_1
  - example_task_2
  - example_task_3
buildvariants:
- name: "bv"
  display_name: "bv_display"
  tasks:
    - name: example_task_group
`
	pp, err = model.LoadProjectInto(ctx, []byte(largeMaxHostYml), nil, "", &proj)
	require.NotNil(t, proj)
	assert.NotNil(pp)
	assert.NoError(err)
	validationErrs = validateTaskGroups(&proj)
	require.Len(t, validationErrs, 1)
	assert.Contains(validationErrs[0].Message, "attach.results cannot be used in the group teardown stage")
	validationErrs = checkTaskGroups(&proj)
	require.Len(t, validationErrs, 1)
	assert.Contains(validationErrs[0].Message, "task group 'example_task_group' has max number of hosts 4 greater than the number of tasks 3")
	assert.Equal(validationErrs[0].Level, Warning)

	overMaxTimeoutYml := `
tasks:
- name: example_task_1
task_groups:
- name: example_task_group
  max_hosts: 4
  teardown_group_timeout_secs: 1800
  tasks:
  - example_task_1
buildvariants:
- name: "bv"
  display_name: "bv_display"
  tasks:
    - name: example_task_group
`
	pp, err = model.LoadProjectInto(ctx, []byte(overMaxTimeoutYml), nil, "", &proj)
	require.NotNil(t, proj)
	assert.NotNil(pp)
	assert.NoError(err)

	validationErrs = checkTaskGroups(&proj)
	require.Len(t, validationErrs, 1)
	assert.Contains(validationErrs[0].Message, "task group 'example_task_group' has a teardown task timeout of 1800 seconds, which exceeds the maximum of 180 seconds")
	assert.Equal(validationErrs[0].Level, Warning)
}

func TestTaskGroupTeardownValidation(t *testing.T) {
	baseYml := `
tasks:
- name: example_task_1
- name: example_task_2

buildvariants:
- name: "bv"
  display_name: "bv_display"
  tasks:
  - name: example_task_group
task_groups:
- name: example_task_group
  setup_group:
  - command: shell.exec
    params:
      script: "echo setup_group"
  tasks:
  - example_task_1
  - example_task_2
`

	var proj model.Project
	ctx := context.Background()
	// verify that attach commands can't be used in teardown group
	for _, commandName := range evergreen.AttachCommands {
		attachCommand := fmt.Sprintf(`
  teardown_group:
  - command: %s
`, commandName)
		attachTeardownYml := fmt.Sprintf("%s\n%s", baseYml, attachCommand)
		pp, err := model.LoadProjectInto(ctx, []byte(attachTeardownYml), nil, "", &proj)
		assert.NotNil(t, proj)
		assert.NotNil(t, pp)
		assert.NoError(t, err)
		validationErrs := validateTaskGroups(&proj)
		assert.Len(t, validationErrs, 1)
		assert.Contains(t, validationErrs[0].Message, fmt.Sprintf("%s cannot be used in the group teardown stage", commandName))
	}

}

func TestTaskNotInTaskGroupDependsOnTaskInTaskGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.Clear(distro.Collection))
	d := distro.Distro{Id: "example_distro"}
	require.NoError(d.Insert(ctx))
	exampleYml := `
exec_timeout_secs: 100
tasks:
- name: not_in_a_task_group
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo test
  depends_on:
  - name: task_in_a_task_group_1
- name: task_in_a_task_group_1
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo test
- name: task_in_a_task_group_2
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo test
task_groups:
- name: example_task_group
  max_hosts: 1
  tasks:
  - task_in_a_task_group_1
  - task_in_a_task_group_2
buildvariants:
- name: "bv"
  display_name: "bv_display"
  run_on: "example_distro"
  tasks:
  - name: not_in_a_task_group
  - name: example_task_group
`
	proj := model.Project{}
	pp, err := model.LoadProjectInto(ctx, []byte(exampleYml), nil, "example_project", &proj)
	assert.NotNil(proj)
	assert.NotNil(pp)
	assert.NoError(err)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("example_task_group", tg.Name)
	assert.Len(tg.Tasks, 2)
	assert.Equal("not_in_a_task_group", proj.Tasks[0].Name)
	assert.Equal("task_in_a_task_group_1", proj.Tasks[0].DependsOn[0].Name)
	errors := CheckProjectErrors(ctx, &proj, false)
	assert.Len(errors, 0)
	warnings := CheckProjectWarnings(&proj)
	assert.Len(warnings, 0)
}

func TestDisplayTaskExecutionTasksNameValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.Clear(distro.Collection))
	d := distro.Distro{Id: "example_distro"}
	require.NoError(d.Insert(ctx))
	exampleYml := `
tasks:
- name: one
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: |
        echo "test"
- name: two
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo "test"
- name: display_three
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo "test"
buildvariants:
- name: "bv"
  display_name: "bv_display"
  run_on: "example_distro"
  tasks:
  - name: one
  - name: two
  display_tasks:
  - name: display_ordinals
    execution_tasks:
    - one
    - two
`
	proj := model.Project{}
	pp, err := model.LoadProjectInto(ctx, []byte(exampleYml), nil, "example_project", &proj)
	assert.NotNil(proj)
	assert.NotNil(pp)
	assert.NoError(err)

	proj.BuildVariants[0].DisplayTasks[0].ExecTasks = append(proj.BuildVariants[0].DisplayTasks[0].ExecTasks,
		"display_three")
	proj.BuildVariants[0].Tasks = append(proj.BuildVariants[0].Tasks, model.BuildVariantTaskUnit{Name: "display_three"})

	errors := CheckProjectErrors(ctx, &proj, false)
	require.Len(errors, 1)
	assert.Equal(errors[0].Level, Error)
	assert.Equal("execution task 'display_three' has prefix 'display_' which is invalid",
		errors[0].Message)
	warnings := CheckProjectWarnings(&proj)
	assert.Len(warnings, 0)
}

func TestValidateCreateHosts(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	// passing case
	yml := `
  tasks:
  - name: t_1
    commands:
    - command: host.create
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - name: t_1
  `
	var p model.Project
	ctx := context.Background()
	pp, err := model.LoadProjectInto(ctx, []byte(yml), nil, "id", &p)
	require.NoError(err)
	require.NotNil(pp)
	errs := validateHostCreates(&p)
	assert.Len(errs, 0)

	// error: times called per task
	yml = `
  tasks:
  - name: t_1
    commands:
    - command: host.create
    - command: host.create
    - command: host.create
    - command: host.create
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - name: t_1
  `
	pp, err = model.LoadProjectInto(ctx, []byte(yml), nil, "id", &p)
	require.NoError(err)
	require.NotNil(pp)
	errs = validateHostCreates(&p)
	assert.Len(errs, 1)
}

func TestValidateParameters(t *testing.T) {
	p := &model.Project{
		Parameters: []model.ParameterInfo{
			{
				Parameter: patch.Parameter{
					Key:   "iter=count",
					Value: "",
				},
			},
		},
	}

	assert.Len(t, validateParameters(p), 1)
	p.Parameters[0].Parameter.Key = ""
	assert.Len(t, validateParameters(p), 1)
	p.Parameters[0].Parameter.Key = "iter_count"
	assert.Len(t, validateParameters(p), 0)
	p.Parameters[0].Description = "not validated"
	p.Parameters[0].Value = "also not"
	assert.Len(t, validateParameters(p), 0)
}

func TestDuplicateTaskInBV(t *testing.T) {
	assert := assert.New(t)

	// a bv with the same task in a task group and by itself should error
	yml := `
  tasks:
  - name: t1
  task_groups:
  - name: tg1
    tasks:
    - t1
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - tg1
    - t1
  `
	var p model.Project
	ctx := context.Background()
	pp, err := model.LoadProjectInto(ctx, []byte(yml), nil, "", &p)
	assert.NoError(err)
	assert.NotNil(pp)
	errs := validateDuplicateBVTasks(&p)
	assert.Len(errs, 1)
	assert.Contains(errs[0].Message, "task 't1' in 'bv' is listed more than once")

	// same as above but reversed in order
	yml = `
  tasks:
  - name: t1
  task_groups:
  - name: tg1
    tasks:
    - t1
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - t1
    - tg1
  `
	pp, err = model.LoadProjectInto(ctx, []byte(yml), nil, "", &p)
	assert.NoError(err)
	assert.NotNil(pp)
	errs = validateDuplicateBVTasks(&p)
	assert.Len(errs, 1)
	assert.Contains(errs[0].Message, "task 't1' in 'bv' is listed more than once")

	// a bv with 2 task groups with the same task should error
	yml = `
  tasks:
  - name: t1
  task_groups:
  - name: tg1
    tasks:
    - t1
  - name: tg2
    tasks:
    - t1
  buildvariants:
  - name: "bv"
    display_name: "bv_display"
    tasks:
    - tg1
    - tg2
  `
	pp, err = model.LoadProjectInto(ctx, []byte(yml), nil, "", &p)
	assert.NoError(err)
	assert.NotNil(pp)
	errs = validateDuplicateBVTasks(&p)
	assert.Len(errs, 1)
	assert.Contains(errs[0].Message, "task 't1' in 'bv' is listed more than once")
}

func TestCheckProjectConfigurationIsValid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.Clear(distro.Collection))
	d := distro.Distro{Id: "example_distro"}
	require.NoError(d.Insert(ctx))
	exampleYml := `
tasks:
- name: one
  commands:
  - command: shell.exec
    params:
      script: |
        echo test
- name: two
  commands:
  - command: shell.exec
    params:
      script: |
        echo test
buildvariants:
- name: "bv-1"
  display_name: "bv_display"
  run_on: "example_distro"
  tasks:
  - name: one
  - name: two
- name: "bv-2"
  display_name: "bv_display"
  run_on: "example_distro"
  tasks:
  - name: one
  - name: two
`
	proj := model.Project{}
	pp, err := model.LoadProjectInto(ctx, []byte(exampleYml), nil, "example_project", &proj)
	require.NoError(err)
	assert.NotEmpty(proj)
	assert.NotNil(pp)
	errs := CheckProjectErrors(ctx, &proj, false)
	assert.Len(errs, 0, "no errors were found")
	errs = CheckProjectWarnings(&proj)
	assert.Len(errs, 2, "two warnings were found")
	assert.NoError(CheckProjectConfigurationIsValid(ctx, &evergreen.Settings{}, &proj, &model.ProjectRef{}), "no errors are reported because they are warnings")

	exampleYml = `
tasks:
  - name: taskA
    commands:
    - command: s3.push
    - command: s3.push
buildvariants:
  - name: bvA
    display_name: "bvA_display"
    run_on: example_distro
    tasks:
      - name: taskA
`
	pp, err = model.LoadProjectInto(ctx, []byte(exampleYml), nil, "example_project", &proj)
	require.NoError(err)
	assert.NotNil(pp)
	assert.NotEmpty(proj)
	assert.Error(CheckProjectConfigurationIsValid(ctx, &evergreen.Settings{}, &proj, &model.ProjectRef{}))
}

func TestGetDistrosForProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.Clear(distro.Collection))
	d1 := distro.Distro{
		Id:            "distro1",
		Aliases:       []string{"distro1-alias", "distro1and2-alias"},
		ValidProjects: []string{"project1", "project2"},
		WarningNote:   "this is a warning for the first distro",
	}
	require.NoError(d1.Insert(ctx))
	d2 := distro.Distro{
		Id:          "distro2",
		Aliases:     []string{"distro2-alias", "distro1and2-alias"},
		WarningNote: "this is the warning for another distro",
	}
	require.NoError(d2.Insert(ctx))
	d3 := distro.Distro{
		Id:            "distro3",
		ValidProjects: []string{"project5"},
	}
	require.NoError(d3.Insert(ctx))

	ids, aliases, warnings, err := getDistros(ctx)
	require.NoError(err)
	require.Len(ids, 3)
	require.Len(aliases, 3)
	require.Len(warnings, 5)
	assert.Contains(aliases, "distro1and2-alias")
	assert.Contains(aliases, "distro1-alias")
	assert.Contains(aliases, "distro2-alias")
	assert.Equal(warnings[d1.Id], d1.WarningNote)
	assert.Equal(warnings[d2.Id], d2.WarningNote)
	assert.Equal(warnings["distro1-alias"], d1.WarningNote)
	assert.Equal(warnings["distro2-alias"], d2.WarningNote)
	assert.Contains(warnings["distro1and2-alias"], d1.WarningNote)
	assert.Contains(warnings["distro1and2-alias"], d2.WarningNote)

	ids, aliases, warnings, err = getDistrosForProject(ctx, "project1")
	require.NoError(err)
	require.Len(ids, 2)
	require.Len(warnings, 5) // Both d1 and d2 are going to match here
	assert.Contains(ids, "distro1")
	assert.Contains(aliases, "distro1and2-alias")
	assert.Contains(aliases, "distro1-alias")

	// Only d2 is going to match here
	ids, aliases, warnings, err = getDistrosForProject(ctx, "project3")
	require.NoError(err)
	require.Len(ids, 1)
	assert.Len(warnings, 3)
	assert.Contains(ids, "distro2")
	assert.Contains(aliases, "distro2-alias")
	assert.Contains(aliases, "distro1and2-alias")
	assert.Equal(warnings[d2.Id], d2.WarningNote)
	assert.Equal(warnings["distro2-alias"], d2.WarningNote)
	assert.Equal(warnings["distro1and2-alias"], d2.WarningNote)
}

func TestValidateTaskSyncCommands(t *testing.T) {
	t.Run("TaskWithNoS3PushCallsPasses", func(t *testing.T) {
		p := &model.Project{
			Tasks: []model.ProjectTask{
				{
					Name:     t.Name(),
					Commands: []model.PluginCommandConf{},
				},
			},
		}
		assert.Empty(t, validateTaskSyncCommands(p, false))
	})
	t.Run("TaskWithMultipleS3PushCallsFails", func(t *testing.T) {
		p := &model.Project{
			Tasks: []model.ProjectTask{
				{
					Name: t.Name(),
					Commands: []model.PluginCommandConf{
						{
							Command: evergreen.S3PushCommandName,
						},
						{
							Command: evergreen.S3PushCommandName,
						},
					},
				},
			},
			BuildVariants: []model.BuildVariant{
				{
					Name: "build_variant",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    t.Name(),
							Variant: "build_variant",
						},
					},
				},
			},
		}
		assert.NotEmpty(t, validateTaskSyncCommands(p, false))
	})
}

func TestValidateVersionControl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := &model.ProjectRef{
		Identifier:            "proj",
		VersionControlEnabled: utility.FalsePtr(),
	}
	projectConfig := model.ProjectConfig{
		Id: "proj",
		ProjectConfigFields: model.ProjectConfigFields{
			BuildBaronSettings: &evergreen.BuildBaronSettings{
				TicketCreateProject:  "ABC",
				TicketSearchProjects: []string{"EVG"},
			},
		},
	}
	isConfigDefined := &projectConfig != nil
	verrs := validateVersionControl(ctx, &evergreen.Settings{}, &model.Project{}, ref, isConfigDefined)
	assert.Equal(t, "version control is disabled for project 'proj'; the currently defined project config fields will not be picked up", verrs[0].Message)

	ref.VersionControlEnabled = utility.TruePtr()
	verrs = validateVersionControl(ctx, &evergreen.Settings{}, &model.Project{}, ref, false)
	assert.Equal(t, "version control is enabled for project 'proj' but no project config fields have been set.", verrs[0].Message)

}

func TestValidateContainers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				Pod: evergreen.AWSPodConfig{
					ECS: evergreen.ECSConfig{
						AllowedImages: []string{
							"hadjri/evg-container-self-tests",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, evergreen.UpdateConfig(ctx, testutil.TestConfig()))
	defer func() {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, p *model.Project, ref *model.ProjectRef){
		"SucceedsWithValidProjectAndRef": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			verrs := validateContainers(ctx, s, p, ref, false)
			assert.Len(t, verrs, 0)
		},
		"FailsWithoutContainerName": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Name = ""
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "name must be defined")
		},
		"FailsWithoutContainerImage": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Image = ""
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "image must be defined")
		},
		"FailsWithoutContainerWorkingDirectory": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].WorkingDir = ""
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "working directory must be defined")
		},
		"FailsWithNotAllowedImage": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Image = "not_allowed"
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "image 'not_allowed' not allowed")
		},
		"MustSpecifyEitherContainerSizeOrResources": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Size = ""
			p.Containers[0].Resources = nil
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "either size or resources must be defined")
		},
		"ContainerSizeAndResourcesAreMutuallyExclusive": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Resources = &model.ContainerResources{
				MemoryMB: 100,
				CPU:      1,
			}
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "size and resources cannot both be defined")
		},
		"FailsWithNonexistentContainerSize": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Size = "s2"
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "container size 's2' not found")
		},
		"FailsWithNonexistentRepoCred": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Credential = "nonexistent"
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "credential 'nonexistent' is not defined in project settings")
		},
		"FailsWithInvalidOSAndArch": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].System = model.ContainerSystem{
				OperatingSystem: "oops",
				CPUArchitecture: "oops",
			}
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "unrecognized container OS 'oops'")
			assert.Contains(t, verrs[0].Message, "unrecognized CPU architecture 'oops'")
		},
		"FailsWithInvalidContainerResources": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			p.Containers[0].Resources = &model.ContainerResources{
				MemoryMB: 0,
				CPU:      -1,
			}
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "container resource CPU must be a positive integer")
			assert.Contains(t, verrs[0].Message, "container resource memory MB must be a positive integer")
		},
		"FailsWithPodSecretAsReferencedRepoCred": func(t *testing.T, p *model.Project, ref *model.ProjectRef) {
			ref.ContainerSecrets[0].Type = model.ContainerSecretPodSecret
			verrs := validateContainers(ctx, s, p, ref, false)
			require.Len(t, verrs, 1)
			assert.Contains(t, verrs[0].Message, "container credential named 'c1' exists but is not valid for use as a repository credential")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection))

			p := &model.Project{
				Identifier: "proj",
				Containers: []model.Container{
					{
						Name:       "c1",
						Image:      "${image}",
						WorkingDir: "/root",
						Size:       "s1",
						Credential: "c1",
					},
				},
			}

			projVars := model.ProjectVars{
				Id:   "proj",
				Vars: map[string]string{"image": "hadjri/evg-container-self-tests"},
			}

			ref := &model.ProjectRef{
				Id:         "proj",
				Identifier: "proj",
				ContainerSizeDefinitions: []model.ContainerResources{
					{
						Name:     "s1",
						CPU:      1,
						MemoryMB: 100,
					},
					{
						Name:     "",
						CPU:      1,
						MemoryMB: 100,
					},
				},
				ContainerSecrets: []model.ContainerSecret{
					{
						Name:       "c1",
						ExternalID: "external_id",
						Type:       model.ContainerSecretRepoCreds,
					},
				},
			}

			require.NoError(t, ref.Upsert())
			require.NoError(t, projVars.Insert())
			tCase(t, p, ref)
		})
	}
}

func TestValidateTaskSyncSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testParams := range map[string]struct {
		tasks                    []model.ProjectTask
		taskSyncEnabledForConfig bool
		expectError              bool
	}{
		"NoTaskSyncPasses": {
			expectError: false,
		},
		"ConfigWithTaskSyncWhenEnabledPasses": {
			taskSyncEnabledForConfig: true,
			tasks: []model.ProjectTask{
				{
					Commands: []model.PluginCommandConf{
						{
							Command: evergreen.S3PushCommandName,
						},
					},
				},
			},
			expectError: false,
		},
		"ConfigWithS3PushWhenDisabledFails": {
			tasks: []model.ProjectTask{
				{
					Commands: []model.PluginCommandConf{
						{
							Command: evergreen.S3PushCommandName,
						},
					},
				},
			},
			expectError: true,
		},
		"ConfigWithS3PullWhenDisabledFails": {
			tasks: []model.ProjectTask{
				{
					Commands: []model.PluginCommandConf{
						{
							Command: evergreen.S3PullCommandName,
						},
					},
				},
			},
			expectError: true,
		},
		"ConfigWithoutTaskSyncWhenEnabledPasses": {
			taskSyncEnabledForConfig: true,
			expectError:              false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ref := &model.ProjectRef{
				TaskSync: model.TaskSyncOptions{
					ConfigEnabled: &testParams.taskSyncEnabledForConfig,
				},
			}
			p := &model.Project{Tasks: testParams.tasks}
			errs := validateTaskSyncSettings(ctx, &evergreen.Settings{}, p, ref, false)
			if testParams.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
	ref := &model.ProjectRef{}
	p := &model.Project{
		Tasks: []model.ProjectTask{
			{
				Commands: []model.PluginCommandConf{
					{
						Command: evergreen.S3PushCommandName,
					},
				},
			},
		},
	}
	assert.NotEmpty(t, validateTaskSyncSettings(ctx, &evergreen.Settings{}, p, ref, false))

	ref.TaskSync.ConfigEnabled = utility.TruePtr()
	assert.Empty(t, validateTaskSyncSettings(ctx, &evergreen.Settings{}, p, ref, false))

	p.Tasks = []model.ProjectTask{}
	assert.Empty(t, validateTaskSyncSettings(ctx, &evergreen.Settings{}, p, ref, false))
}

func TestTVToTaskUnit(t *testing.T) {
	for testName, testCase := range map[string]struct {
		expectedTVToTaskUnit map[model.TVPair]model.BuildVariantTaskUnit
		project              model.Project
	}{
		"MapsTasksAndPopulates": {
			expectedTVToTaskUnit: map[model.TVPair]model.BuildVariantTaskUnit{
				{TaskName: "setup", Variant: "rhel"}: {
					Name:     "setup",
					Variant:  "rhel",
					Priority: 20,
				}, {TaskName: "compile", Variant: "ubuntu"}: {
					Name:             "compile",
					Variant:          "ubuntu",
					CommitQueueMerge: true,
					DependsOn: []model.TaskUnitDependency{
						{
							Name:    "setup",
							Variant: "rhel",
						},
					},
				}, {TaskName: "compile", Variant: "suse"}: {
					Name:    "compile",
					Variant: "suse",
					DependsOn: []model.TaskUnitDependency{
						{
							Name:    "setup",
							Variant: "rhel",
						},
					},
				},
			},
			project: model.Project{
				Tasks: []model.ProjectTask{
					{
						Name:            "setup",
						Priority:        10,
						ExecTimeoutSecs: 10,
					}, {
						Name:            "compile",
						ExecTimeoutSecs: 10,
						DependsOn: []model.TaskUnitDependency{
							{
								Name:    "setup",
								Variant: "rhel",
							},
						},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "rhel",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:     "setup",
								Variant:  "rhel",
								Priority: 20,
							},
						},
					}, {
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:             "compile",
								Variant:          "ubuntu",
								CommitQueueMerge: true,
								DependsOn: []model.TaskUnitDependency{
									{
										Name:    "setup",
										Variant: "rhel",
									},
								},
							},
						},
					}, {
						Name: "suse",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "compile",
								Variant: "suse",
								DependsOn: []model.TaskUnitDependency{
									{
										Name:    "setup",
										Variant: "rhel",
									},
								},
							},
						},
					},
				},
			},
		},
		"MapsTaskGroupTasksAndPopulates": {
			expectedTVToTaskUnit: map[model.TVPair]model.BuildVariantTaskUnit{
				{TaskName: "setup", Variant: "rhel"}: {
					Name:     "setup",
					Variant:  "rhel",
					Priority: 20,
				}, {TaskName: "compile", Variant: "ubuntu"}: {
					Name:             "compile",
					Variant:          "ubuntu",
					IsPartOfGroup:    true,
					GroupName:        "compile_group",
					CommitQueueMerge: true,
					DependsOn: []model.TaskUnitDependency{
						{
							Name:    "setup",
							Variant: "rhel",
						},
					},
				}, {TaskName: "compile", Variant: "suse"}: {
					Name:          "compile",
					Variant:       "suse",
					IsPartOfGroup: true,
					GroupName:     "compile_group",
					DependsOn: []model.TaskUnitDependency{
						{
							Name:    "setup",
							Variant: "rhel",
						},
					},
				},
			},
			project: model.Project{
				TaskGroups: []model.TaskGroup{
					{
						Name:  "compile_group",
						Tasks: []string{"compile"},
					},
				},
				Tasks: []model.ProjectTask{
					{
						Name:            "setup",
						Priority:        10,
						ExecTimeoutSecs: 10,
					}, {
						Name:            "compile",
						ExecTimeoutSecs: 10,
						DependsOn: []model.TaskUnitDependency{
							{
								Name:    "setup",
								Variant: "rhel",
							},
						},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "rhel",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:     "setup",
								Variant:  "rhel",
								Priority: 20,
							},
						},
					}, {
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:             "compile_group",
								Variant:          "ubuntu",
								CommitQueueMerge: true,
							},
						},
					}, {
						Name: "suse",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "compile_group",
								Variant: "suse",
							},
						},
					},
				},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tvToTaskUnit := tvToTaskUnit(&testCase.project)
			assert.Len(t, tvToTaskUnit, len(testCase.expectedTVToTaskUnit))
			for expectedTV := range testCase.expectedTVToTaskUnit {
				assert.Contains(t, tvToTaskUnit, expectedTV)
				taskUnit := tvToTaskUnit[expectedTV]
				expectedTaskUnit := testCase.expectedTVToTaskUnit[expectedTV]
				assert.Equal(t, expectedTaskUnit.Name, taskUnit.Name)
				assert.Equal(t, expectedTaskUnit.IsGroup, taskUnit.IsGroup, fmt.Sprintf("%s/%s", expectedTaskUnit.Variant, expectedTaskUnit.Name))
				assert.Equal(t, expectedTaskUnit.IsPartOfGroup, taskUnit.IsPartOfGroup, fmt.Sprintf("%s/%s", expectedTaskUnit.Variant, expectedTaskUnit.Name))
				assert.Equal(t, expectedTaskUnit.GroupName, taskUnit.GroupName, fmt.Sprintf("%s/%s", expectedTaskUnit.Variant, expectedTaskUnit.Name))
				assert.Equal(t, expectedTaskUnit.Patchable, taskUnit.Patchable, expectedTaskUnit.Name)
				assert.Equal(t, expectedTaskUnit.PatchOnly, taskUnit.PatchOnly)
				assert.Equal(t, expectedTaskUnit.Priority, taskUnit.Priority)
				missingActual, missingExpected := utility.StringSliceSymmetricDifference(expectedTaskUnit.RunOn, taskUnit.RunOn)
				assert.Empty(t, missingActual)
				assert.Empty(t, missingExpected)
				assert.Len(t, taskUnit.DependsOn, len(expectedTaskUnit.DependsOn))
				for _, dep := range expectedTaskUnit.DependsOn {
					assert.Contains(t, taskUnit.DependsOn, dep)
				}
				assert.Equal(t, expectedTaskUnit.Stepback, taskUnit.Stepback)
				assert.Equal(t, expectedTaskUnit.CommitQueueMerge, taskUnit.CommitQueueMerge, fmt.Sprintf("%s/%s", expectedTaskUnit.Variant, expectedTaskUnit.Name))
				assert.Equal(t, expectedTaskUnit.Variant, taskUnit.Variant)
			}
		})
	}
}

func TestValidateTVDependsOnTV(t *testing.T) {
	for testName, testCase := range map[string]struct {
		dependedOnTask model.TVPair
		dependentTask  model.TVPair
		statuses       []string
		buildVariants  []model.BuildVariant
		expectError    bool
	}{
		"FindsDependency": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{Name: "B"},
							},
						},
						{
							Name:    "B",
							Variant: "ubuntu",
						},
					},
				},
			},
			expectError: false,
		},
		"FindsDependencyWithoutExplicitBV": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							DependsOn: []model.TaskUnitDependency{{Name: "B"}},
						},
						{
							Name:    "B",
							Variant: "ubuntu",
						},
					},
				},
			},
			expectError: false,
		},
		"FindsDependencyTransitively": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name: "B",
								},
							},
						},
						{
							Name:    "B",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "C",
									Variant: "rhel",
								},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "C",
							Variant: "rhel",
						},
					},
				},
			},
			expectError: false,
		},
		"FailsForNoDependency": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
						},
					},
				},
			},
			expectError: true,
		},
		"FailsIfDependencySkipsPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "B",
									Variant: "ubuntu",
								},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
						},
					},
				},
			},
			expectError: true,
		},
		"FailsIfIntermediateDependencySkipsPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name: "B",
								},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "C", Variant: "rhel"},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{Name: "C", Variant: "rhel"},
					},
				},
			},
			expectError: true,
		},
		"FailsIfDependencySkipsNonPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "B",
									Variant: "ubuntu",
								},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
						},
					},
				},
			},
			expectError: true,
		},
		"FailsIfIntermediateDependencySkipsNonPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name: "B",
								},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							PatchOnly: utility.TruePtr(),
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "C",
									Variant: "rhel",
								},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "C",
							Variant: "rhel",
						},
					},
				},
			},
			expectError: true,
		},
		"FailsIfDependencyIsPatchOptional": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:          "B",
									Variant:       "ubuntu",
									PatchOptional: true,
								},
							},
						},
						{
							Name:    "B",
							Variant: "ubuntu",
						},
					},
				},
			},
			expectError: true,
		},
		"FailsIfIntermediateDependencyIsPatchOptional": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:    "B",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{Name: "C", Variant: "rhel", PatchOptional: true},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "C",
							Variant: "rhel",
						},
					},
				},
			},
			expectError: true,
		},
		"OnlyLastDependencyRequiresSuccessStatus": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			statuses: []string{
				evergreen.TaskSucceeded,
				"",
			},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Status: evergreen.TaskFailed},
							},
						},
						{
							Name:    "B",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{Name: "C", Variant: "rhel"},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "C",
							Variant: "rhel",
						},
					},
				},
			},
			expectError: false,
		},
		"FailsIfDependencyDoesNotRequireSuccessStatus": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			statuses: []string{
				evergreen.TaskSucceeded,
				"",
			},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "B",
									Variant: "ubuntu",
									Status:  evergreen.TaskFailed,
								},
							},
						},
						{Name: "B", Variant: "ubuntu"},
					},
				},
			},
			expectError: true,
		},
		"FailsIfLastDependencyDoesNotRequireSuccessStatus": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			statuses: []string{
				evergreen.TaskSucceeded,
				"",
			},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name: "B",
								},
							},
						},
						{
							Name:    "B",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{
									Name:    "C",
									Variant: "rhel",
									Status:  evergreen.TaskFailed,
								},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "C",
							Variant: "rhel",
						},
					},
				},
			},
			expectError: true,
		},
		"DependencyCanSkipPatchesIfSourceSkipsPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
						},
					},
				},
			},
			expectError: false,
		},
		"IntermediateDependencyCanSkipPatchesIfSourceSkipsPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "B"},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "C", Variant: "rhel", Status: evergreen.TaskFailed},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "C",
							Variant: "rhel",
						},
					},
				},
			},
			expectError: false,
		},
		"DependencyCanSkipNonPatchesIfSourceSkipsNonPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							PatchOnly: utility.TruePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							PatchOnly: utility.TruePtr(),
						},
					},
				},
			},
			expectError: false,
		},
		"IntermediateDependencyCanSkipNonPatchesIfSourceSkipsNonPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "C", Variant: "rhel"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							PatchOnly: utility.TruePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "B"},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							PatchOnly: utility.TruePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "C", Variant: "rhel"},
							},
						},
					},
				},
				{
					Name: "rhel",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "C",
							Variant: "rhel",
						},
					},
				},
			},
			expectError: false,
		},
		"DependencySkipsGitTagsIfSourceRequiresPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							PatchOnly: utility.TruePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:       "B",
							Variant:    "ubuntu",
							GitTagOnly: utility.TruePtr(),
						},
					},
				},
			},
			expectError: true,
		},
		"DependencySkipsGitTagsIfSourceRequiresNonPatches": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:       "B",
							Variant:    "ubuntu",
							GitTagOnly: utility.TruePtr(),
						},
					},
				},
			},
			expectError: true,
		},
		"DependencySkipsGitTagsIfNotAllowedForGitTags": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:      "A",
							Variant:   "ubuntu",
							Patchable: utility.FalsePtr(),
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:           "B",
							Variant:        "ubuntu",
							AllowForGitTag: utility.FalsePtr(),
						},
					},
				},
			},
			expectError: true,
		},
		"DependencyIncludesGitTagsIfAllowed": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:           "B",
							Variant:        "ubuntu",
							AllowForGitTag: utility.TruePtr(),
						},
					},
				},
			},
			expectError: false,
		},
		"DependencySkipsPatchIfSourceIncludesGitTags": {
			dependentTask:  model.TVPair{TaskName: "A", Variant: "ubuntu"},
			dependedOnTask: model.TVPair{TaskName: "B", Variant: "ubuntu"},
			buildVariants: []model.BuildVariant{
				{
					Name: "ubuntu",
					Tasks: []model.BuildVariantTaskUnit{
						{
							Name:    "A",
							Variant: "ubuntu",
							DependsOn: []model.TaskUnitDependency{
								{Name: "B", Variant: "ubuntu"},
							},
						},
						{
							Name:      "B",
							Variant:   "ubuntu",
							PatchOnly: utility.TruePtr(),
						},
					},
				},
			},
			expectError: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			err := validateTVDependsOnTV(
				testCase.dependentTask,
				testCase.dependedOnTask,
				testCase.statuses,
				&model.Project{BuildVariants: testCase.buildVariants},
			)
			if testCase.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseS3PullParameters(t *testing.T) {
	for testName, testCase := range map[string]struct {
		expectError bool
		params      map[string]interface{}
	}{
		"PassesWithPopulatedParameters": {
			expectError: false,
			params: map[string]interface{}{
				"task":               "t",
				"from_build_variant": "bv",
			},
		},
		"PassesWithPopulatedTaskOnly": {
			expectError: false,
			params: map[string]interface{}{
				"task": "t",
			},
		},
		"FailsForEmptyParameters": {
			expectError: true,
			params:      map[string]interface{}{},
		},
		"FailsForNilParameters": {
			expectError: true,
		},
		"FailsForMissingTask": {
			expectError: true,
			params: map[string]interface{}{
				"from_build_variant": "bv",
			},
		},
		"FailsForNonStringTaskArgument": {
			expectError: true,
			params: map[string]interface{}{
				"task":               0,
				"from_build_variant": "bv",
			},
		},
		"FailsForNonStringBuildVariantArgument": {
			expectError: true,
			params: map[string]interface{}{
				"task":               "task",
				"from_build_variant": 0,
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			cmd := model.PluginCommandConf{
				Command: evergreen.S3PullCommandName,
				Params:  testCase.params,
			}
			task, bv, err := parseS3PullParameters(cmd)
			if testCase.expectError {
				assert.Error(t, err)
				assert.Empty(t, task)
				assert.Empty(t, bv)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.params["task"], task)
				if fromBV, ok := testCase.params["from_build_variant"]; ok {
					assert.Equal(t, fromBV, bv)
				} else {
					assert.Empty(t, bv)
				}
			}
		})
	}
}

func TestValidateTaskGroupsInBV(t *testing.T) {
	tests := map[string]struct {
		project        model.Project
		expectErr      bool
		expectedErrMsg string
	}{
		"Task group before task": {
			project: model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "task1",
					},
					{
						Name: "task2",
					},
					{
						Name: "task3",
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "task1-and-task2",
						Tasks: []string{"task1", "task2"},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "task1-and-task2", Variant: "ubuntu", IsGroup: true},
							{Name: "task1", Variant: "ubuntu"},
						},
					},
				},
			},
			expectErr:      true,
			expectedErrMsg: "task 'task1' in build variant 'ubuntu' is already referenced in task group 'task1-and-task2'",
		},
		"Task group after task": {
			project: model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "task1",
					},
					{
						Name: "task2",
					},
					{
						Name: "task3",
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "task1-and-task2",
						Tasks: []string{"task1", "task2"},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "task2", Variant: "ubuntu"},
							{Name: "task1-and-task2", Variant: "ubuntu", IsGroup: true},
						},
					},
				},
			},
			expectErr:      true,
			expectedErrMsg: "task 'task2' in build variant 'ubuntu' is already referenced in task group 'task1-and-task2'",
		},
		"Task group and task not in task group": {
			project: model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "task1",
					},
					{
						Name: "task2",
					},
					{
						Name: "task3",
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "task1-and-task2",
						Tasks: []string{"task1", "task2"},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "task3", Variant: "ubuntu"},
							{Name: "task1-and-task2", Variant: "ubuntu", IsGroup: true},
						},
					},
				},
			},
			expectErr: false,
		},
		"No task group": {
			project: model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "task1",
					},
					{
						Name: "task2",
					},
					{
						Name: "task3",
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "task1-and-task2",
						Tasks: []string{"task1", "task2"},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "task3", Variant: "ubuntu"},
							{Name: "task1", Variant: "ubuntu"},
						},
					},
				},
			},
			expectErr:      true,
			expectedErrMsg: "task 'task1' in build variant 'ubuntu' is already referenced in task group 'task1-and-task2'",
		},
		"Multiple task group": {
			project: model.Project{
				Tasks: []model.ProjectTask{
					{
						Name: "task1",
					},
					{
						Name: "task2",
					},
					{
						Name: "task3",
					},
				},
				TaskGroups: []model.TaskGroup{
					{
						Name:  "task1-and-task2",
						Tasks: []string{"task1", "task2"},
					},
					{
						Name:  "task1-and-task3",
						Tasks: []string{"task1", "task3"},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{Name: "task1-and-task2", Variant: "ubuntu", IsGroup: true},
							{Name: "task1-and-task3", Variant: "ubuntu", IsGroup: true},
						},
					},
				},
			},
			expectErr: false,
		},
	}
	for testName, testCase := range tests {
		t.Run(testName, func(t *testing.T) {
			errs := ensureReferentialIntegrity(&testCase.project, nil, []string{}, []string{}, nil)
			if testCase.expectErr {
				assert.Equal(t, errs[0].Message, testCase.expectedErrMsg)
			} else {
				assert.Equal(t, len(errs), 0, "there was an error validating task group in build variant")
			}
		})
	}
}

func TestBVsWithTasksThatCallCommand(t *testing.T) {
	findCmdByDisplayName := func(cmds []model.PluginCommandConf, name string) *model.PluginCommandConf {
		for _, cmd := range cmds {
			if cmd.DisplayName == name {
				return &cmd
			}
		}
		return nil
	}
	cmd := evergreen.S3PullCommandName
	t.Run("CommandsIn", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			project                    model.Project
			expectedBVsToTasksWithCmds map[string]map[string][]model.PluginCommandConf
		}{
			"Task": {
				project: model.Project{
					Tasks: []model.ProjectTask{
						{
							Name: "setup",
							Commands: []model.PluginCommandConf{
								{
									DisplayName: "push_dir",
									Command:     evergreen.S3PushCommandName,
								},
							},
						}, {
							Name: "pull",
							Commands: []model.PluginCommandConf{
								{
									DisplayName: "pull_dir",
									Command:     evergreen.S3PullCommandName,
								},
							},
						}, {
							Name: "pull_twice",
							Commands: []model.PluginCommandConf{
								{
									DisplayName: "pull_dir1",
									Command:     evergreen.S3PullCommandName,
								},
								{
									DisplayName: "pull_dir2",
									Command:     evergreen.S3PullCommandName,
								},
							},
						}, {
							Name: "test",
							Commands: []model.PluginCommandConf{
								{
									DisplayName: "pull_dir_for_test",
									Command:     evergreen.S3PullCommandName,
									Variants:    []string{"rhel", "debian"},
								},
								{
									DisplayName: "generate_test",
									Command:     evergreen.GenerateTasksCommandName,
								},
							},
						}, {
							Name: "lint",
							Commands: []model.PluginCommandConf{
								{
									DisplayName: "generate_lint",
									Command:     evergreen.GenerateTasksCommandName,
								},
							},
						},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "pull",
									Variant: "ubuntu",
								},
							},
						},
						{
							Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test",
									Variant: "rhel",
								},
								{
									Name:    "pull_twice",
									Variant: "rhel",
								},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "lint",
									Variant: "archlinux",
								},
							},
						}, {
							Name: "debian",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "pull",
									Variant: "debian",
								},
								{
									Name:    "test",
									Variant: "debian",
								},
								{
									Name:    "lint",
									Variant: "debian",
								},
							},
						}, {
							Name: "fedora",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "pull",
									Variant: "fedora",
								},
								{
									Name:    "test",
									Variant: "fedora",
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"pull": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir_for_test",
								Command:     evergreen.S3PullCommandName,
							},
						},
						"pull_twice": {
							{
								DisplayName: "pull_dir1",
								Command:     evergreen.S3PullCommandName,
							},
							{
								DisplayName: "pull_dir2",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"debian": {
						"pull": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
						"test": {
							{
								DisplayName: "pull_dir_for_test",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"fedora": {
						"pull": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"TaskFunctionExpandsCommands": {
				project: model.Project{
					Functions: map[string]*model.YAMLCommandSet{
						"pull_func": {
							SingleCommand: &model.PluginCommandConf{
								Command:     evergreen.S3PullCommandName,
								DisplayName: "pull_dir",
							},
						},
						"test_func": {
							MultiCommand: []model.PluginCommandConf{
								{
									Command:     evergreen.S3PullCommandName,
									DisplayName: "pull_dir_for_test",
								}, {
									Command:     evergreen.GenerateTasksCommandName,
									DisplayName: "generate_test",
								},
							},
						},
					},
					Tasks: []model.ProjectTask{
						{
							Name: "setup",
							Commands: []model.PluginCommandConf{
								{
									Function: "pull_func",
								},
							},
						}, {
							Name: "test",
							Commands: []model.PluginCommandConf{
								{
									Function: "test_func",
								},
							},
						},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "setup",
									Variant: "ubuntu",
								},
							},
						},
						{
							Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test",
									Variant: "rhel",
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"setup": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir_for_test",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"Pre": {
				project: model.Project{
					Pre: &model.YAMLCommandSet{
						MultiCommand: []model.PluginCommandConf{
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
								Variants:    []string{"ubuntu", "rhel"},
							},
						},
					},
					Tasks: []model.ProjectTask{
						{Name: "test"},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test",
									Variant: "ubuntu",
								},
							},
						},
						{Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test",
									Variant: "rhel",
								},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test",
									Variant: "archlinux",
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"Post": {
				project: model.Project{
					Post: &model.YAMLCommandSet{
						MultiCommand: []model.PluginCommandConf{
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
								Variants:    []string{"ubuntu", "rhel"},
							},
						},
					},
					Tasks: []model.ProjectTask{
						{Name: "test"},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{Name: "test", Variant: "ubuntu"},
							},
						},
						{Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{Name: "test", Variant: "rhel"},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{Name: "test", Variant: "archlinux"},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"SetupGroupInTaskGroup": {
				project: model.Project{
					TaskGroups: []model.TaskGroup{
						{
							Name: "test_group",
							SetupGroup: &model.YAMLCommandSet{
								MultiCommand: []model.PluginCommandConf{
									{
										DisplayName: "pull_dir",
										Command:     evergreen.S3PullCommandName,
										Variants:    []string{"ubuntu", "rhel"},
									},
								},
							},
							Tasks: []string{"test"},
						},
					},
					Tasks: []model.ProjectTask{
						{Name: "test"},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "ubuntu",
									IsGroup: true,
								},
							},
						},
						{Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "rhel",
									IsGroup: true,
								},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "archlinux",
									IsGroup: true,
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"SetupTaskInTaskGroup": {
				project: model.Project{
					TaskGroups: []model.TaskGroup{
						{
							Name: "test_group",
							SetupTask: &model.YAMLCommandSet{
								MultiCommand: []model.PluginCommandConf{
									{
										DisplayName: "pull_dir",
										Command:     evergreen.S3PullCommandName,
										Variants:    []string{"ubuntu", "rhel"},
									},
								},
							},
							Tasks: []string{"test"},
						},
					},
					Tasks: []model.ProjectTask{
						{Name: "test"},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "ubuntu",
									IsGroup: true,
								},
							},
						},
						{Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "rhel",
									IsGroup: true,
								},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "archlinux",
									IsGroup: true,
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"TasksInTaskGroup": {
				project: model.Project{
					TaskGroups: []model.TaskGroup{
						{
							Name:  "test_group",
							Tasks: []string{"test"},
						},
					},
					Tasks: []model.ProjectTask{
						{
							Name: "test",
							Commands: []model.PluginCommandConf{
								{
									Command:     evergreen.S3PullCommandName,
									DisplayName: "pull_dir",
									Variants:    []string{"ubuntu", "rhel"},
								},
							},
						},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "ubuntu",
									IsGroup: true,
								},
							},
						},
						{Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "rhel",
									IsGroup: true,
								},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "archlinux",
									IsGroup: true,
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"TeardownGroupInTaskGroup": {
				project: model.Project{
					TaskGroups: []model.TaskGroup{
						{
							Name: "test_group",
							TeardownGroup: &model.YAMLCommandSet{
								MultiCommand: []model.PluginCommandConf{
									{
										DisplayName: "pull_dir",
										Command:     evergreen.S3PullCommandName,
										Variants:    []string{"ubuntu", "rhel"},
									},
								},
							},
							Tasks: []string{"test"},
						},
					},
					Tasks: []model.ProjectTask{
						{Name: "test"},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "ubuntu",
									IsGroup: true,
								},
							},
						},
						{Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "rhel",
									IsGroup: true,
								},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "archlinux",
									IsGroup: true,
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
			"TeardownTaskInTaskGroup": {
				project: model.Project{
					TaskGroups: []model.TaskGroup{
						{
							Name: "test_group",
							TeardownTask: &model.YAMLCommandSet{
								MultiCommand: []model.PluginCommandConf{
									{
										DisplayName: "pull_dir",
										Command:     evergreen.S3PullCommandName,
										Variants:    []string{"ubuntu", "rhel"},
									},
								},
							},
							Tasks: []string{"test"},
						},
					},
					Tasks: []model.ProjectTask{
						{Name: "test"},
					},
					BuildVariants: []model.BuildVariant{
						{
							Name: "ubuntu",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "ubuntu",
									IsGroup: true,
								},
							},
						}, {
							Name: "rhel",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "rhel",
									IsGroup: true,
								},
							},
						}, {
							Name: "archlinux",
							Tasks: []model.BuildVariantTaskUnit{
								{
									Name:    "test_group",
									Variant: "archlinux",
									IsGroup: true,
								},
							},
						},
					},
				},
				expectedBVsToTasksWithCmds: map[string]map[string][]model.PluginCommandConf{
					"ubuntu": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
					"rhel": {
						"test": {
							{
								DisplayName: "pull_dir",
								Command:     evergreen.S3PullCommandName,
							},
						},
					},
				},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				bvsToTasksWithCmds, _, err := bvsWithTasksThatCallCommand(&testCase.project, cmd)
				require.NoError(t, err)
				assert.Len(t, bvsToTasksWithCmds, len(testCase.expectedBVsToTasksWithCmds))
				for bv, expectedTasks := range testCase.expectedBVsToTasksWithCmds {
					assert.Contains(t, bvsToTasksWithCmds, bv)
					tasks := bvsToTasksWithCmds[bv]
					assert.Len(t, tasks, len(expectedTasks))
					for taskName, expectedCmds := range expectedTasks {
						assert.Contains(t, tasks, taskName)
						cmds := tasks[taskName]
						assert.Len(t, cmds, len(expectedCmds))
						for _, expectedCmd := range expectedCmds {
							cmd := findCmdByDisplayName(cmds, expectedCmd.DisplayName)
							require.NotNil(t, cmd)
							assert.Equal(t, expectedCmd.Command, cmd.Command)
						}
					}
				}
			})
		}
	})

	t.Run("MissingDefinition", func(t *testing.T) {
		for testName, project := range map[string]model.Project{
			"ForTaskReferencedInBV": {
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "test",
								Variant: "ubuntu",
							},
						},
					},
				},
			},
			"ForTaskGroupReferencedInBV": {
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "test_group",
								Variant: "ubuntu",
								IsGroup: true,
							},
						},
					},
				},
			},
			"ForTaskReferencedInTaskGroupInBV": {
				TaskGroups: []model.TaskGroup{
					{
						Name:  "test_group",
						Tasks: []string{"test"},
					},
				},
				BuildVariants: []model.BuildVariant{
					{
						Name: "ubuntu",
						Tasks: []model.BuildVariantTaskUnit{
							{
								Name:    "test_group",
								Variant: "ubuntu",
								IsGroup: true,
							},
						},
					},
				},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				_, _, err := bvsWithTasksThatCallCommand(&project, cmd)
				assert.Error(t, err)
			})
		}
	})
}

func TestValidationErrorsAtLevel(t *testing.T) {
	t.Run("FindsWarningLevelErrors", func(t *testing.T) {
		errs := ValidationErrors([]ValidationError{
			{
				Level:   Warning,
				Message: "warning",
			}, {
				Level:   Error,
				Message: "error",
			},
		})
		foundErrs := errs.AtLevel(Warning)
		require.Len(t, foundErrs, 1)
		assert.Equal(t, errs[0], foundErrs[0])
	})
	t.Run("FindsErrorLevelErrors", func(t *testing.T) {
		errs := ValidationErrors([]ValidationError{
			{
				Level:   Warning,
				Message: "warning",
			}, {
				Level:   Error,
				Message: "error",
			},
		})
		foundErrs := errs.AtLevel(Error)
		require.Len(t, foundErrs, 1)
		assert.Equal(t, errs[1], foundErrs[0])
	})
	t.Run("ReturnsEmptyForNonexistent", func(t *testing.T) {
		errs := ValidationErrors([]ValidationError{})
		assert.Empty(t, errs.AtLevel(Error))
	})
	t.Run("ReturnsEmptyForNoMatch", func(t *testing.T) {
		errs := ValidationErrors([]ValidationError{
			{
				Level:   Warning,
				Message: "warning",
			},
		})
		assert.Empty(t, errs.AtLevel(Error))
	})
}
