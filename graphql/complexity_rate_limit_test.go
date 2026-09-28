package graphql

import (
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/alicebob/miniredis/v2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

func setupComplexityRateLimitEnv(t *testing.T, cfg evergreen.RateLimitConfig) *mock.Environment {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { assert.NoError(t, rdb.Close()) })

	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	env.SetRedisClient(rdb)
	env.EvergreenSettings.RateLimit = cfg

	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
	return env
}

func parseQuery(t *testing.T, schema graphql.ExecutableSchema, queryStr string) *ast.OperationDefinition {
	doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), queryStr, rules.NewDefaultRules())
	require.Empty(t, gqlErrs)
	require.Len(t, doc.Operations, 1)
	return doc.Operations[0]
}

func TestComplexityRateLimitUnderBudgetPasses(t *testing.T) {
	env := setupComplexityRateLimitEnv(t, evergreen.RateLimitConfig{
		GraphQLComplexityPerHour: 10000,
		GraphQLComplexityBurst:   10000,
	})
	schema := NewExecutableSchema(New(""))
	ext := NewComplexityRateLimit(env, schema)

	op := parseQuery(t, schema, userSettingsQuery)
	opCtx := &graphql.OperationContext{
		Operation: op,
		Doc:       &ast.QueryDocument{Operations: ast.OperationList{op}},
	}
	ctx := graphql.WithOperationContext(t.Context(), opCtx)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})

	gqlErr := ext.MutateOperationContext(ctx, opCtx)
	assert.Nil(t, gqlErr)
}

func TestComplexityRateLimitExceedsBudgetRejects(t *testing.T) {
	env := setupComplexityRateLimitEnv(t, evergreen.RateLimitConfig{
		GraphQLComplexityPerHour: 5,
		GraphQLComplexityBurst:   5,
	})
	schema := NewExecutableSchema(New(""))
	ext := NewComplexityRateLimit(env, schema)

	op := parseQuery(t, schema, hostEventsQuery)
	opCtx := &graphql.OperationContext{
		Operation: op,
		Doc:       &ast.QueryDocument{Operations: ast.OperationList{op}},
	}
	ctx := graphql.WithOperationContext(t.Context(), opCtx)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})

	gqlErr := ext.MutateOperationContext(ctx, opCtx)
	require.NotNil(t, gqlErr)
	assert.Contains(t, gqlErr.Message, "complexity rate limit exceeded")
}

func TestComplexityRateLimitExemptUserPassesThrough(t *testing.T) {
	env := setupComplexityRateLimitEnv(t, evergreen.RateLimitConfig{
		GraphQLComplexityPerHour: 1,
		GraphQLComplexityBurst:   1,
		ExemptUserIDs:            []string{"exempt_user"},
	})
	schema := NewExecutableSchema(New(""))
	ext := NewComplexityRateLimit(env, schema)

	op := parseQuery(t, schema, hostEventsQuery)
	opCtx := &graphql.OperationContext{
		Operation: op,
		Doc:       &ast.QueryDocument{Operations: ast.OperationList{op}},
	}
	ctx := graphql.WithOperationContext(t.Context(), opCtx)
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "exempt_user"})

	gqlErr := ext.MutateOperationContext(ctx, opCtx)
	assert.Nil(t, gqlErr)
}

func TestComplexityRateLimitElevatedUserGetsDoubleBudget(t *testing.T) {
	env := setupComplexityRateLimitEnv(t, evergreen.RateLimitConfig{
		GraphQLComplexityPerHour: 15,
		GraphQLComplexityBurst:   15,
		ElevatedUserIDs:          []string{"elevated_user"},
	})
	schema := NewExecutableSchema(New(""))
	ext := NewComplexityRateLimit(env, schema)

	op := parseQuery(t, schema, hostEventsQuery)
	opCtx := &graphql.OperationContext{
		Operation: op,
		Doc:       &ast.QueryDocument{Operations: ast.OperationList{op}},
	}

	normalCtx := graphql.WithOperationContext(t.Context(), opCtx)
	normalCtx = gimlet.AttachUser(normalCtx, &user.DBUser{Id: "normal_user"})
	gqlErr := ext.MutateOperationContext(normalCtx, opCtx)
	require.NotNil(t, gqlErr)

	elevatedCtx := graphql.WithOperationContext(t.Context(), opCtx)
	elevatedCtx = gimlet.AttachUser(elevatedCtx, &user.DBUser{Id: "elevated_user"})
	gqlErr = ext.MutateOperationContext(elevatedCtx, opCtx)
	assert.Nil(t, gqlErr)
}
