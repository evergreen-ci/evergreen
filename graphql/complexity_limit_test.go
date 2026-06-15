package graphql

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

func TestComplexityLimitMutateOperationContext(t *testing.T) {
	schema := NewExecutableSchema(New(""))

	parseOp := func(t *testing.T, queryStr string) *ast.OperationDefinition {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), queryStr, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		require.Len(t, doc.Operations, 1)
		return doc.Operations[0]
	}

	op := parseOp(t, userSettingsQuery)
	baseline := complexity.Calculate(t.Context(), schema, op, nil)
	require.Greater(t, baseline, 0)

	// run invokes the extension with a recording response writer in the context
	// so the response headers it sets can be asserted.
	run := func(t *testing.T) (*gqlerror.Error, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		ctx := context.WithValue(t.Context(), responseWriterContextKey{}, rec)
		rc := &graphql.OperationContext{Operation: op}
		gqlErr := MakeComplexityLimit(schema).MutateOperationContext(ctx, rc)
		return gqlErr, rec
	}

	for name, test := range map[string]func(t *testing.T){
		"AllowsQueryUnderLimit": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline + 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))

			gqlErr, rec := run(t)
			assert.Nil(t, gqlErr)
			assert.Equal(t, strconv.Itoa(baseline), rec.Header().Get(evergreen.GraphQLComplexityHeader))
			assert.Empty(t, rec.Header().Get(evergreen.GraphQLComplexityExceededHeader))
		},
		"RejectsQueryOverLimit": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline - 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))

			gqlErr, rec := run(t)
			require.NotNil(t, gqlErr)
			assert.Equal(t, ComplexityLimitExceeded, gqlErr.Extensions["code"])
			assert.Equal(t, strconv.Itoa(baseline), rec.Header().Get(evergreen.GraphQLComplexityHeader))
			assert.Equal(t, "true", rec.Header().Get(evergreen.GraphQLComplexityExceededHeader))
		},
		"WarnsButServesWhenLimiterDisabled": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline - 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))
			settings.ServiceFlags.GraphQLComplexityLimiterDisabled = true
			require.NoError(t, settings.ServiceFlags.Set(t.Context()))

			gqlErr, rec := run(t)
			assert.Nil(t, gqlErr)
			// The exceeded header is still set in warn-only mode.
			assert.Equal(t, "true", rec.Header().Get(evergreen.GraphQLComplexityExceededHeader))
		},
		"ZeroLimitDisablesEnforcement": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = 0
			require.NoError(t, settings.RateLimit.Set(t.Context()))

			gqlErr, rec := run(t)
			assert.Nil(t, gqlErr)
			assert.Empty(t, rec.Header().Get(evergreen.GraphQLComplexityExceededHeader))
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
			test(t)
		})
	}
}
