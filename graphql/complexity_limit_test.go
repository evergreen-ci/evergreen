package graphql

import (
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
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

	run := func(t *testing.T) *gqlerror.Error {
		rc := &graphql.OperationContext{Operation: op}
		return MakeComplexityLimit(schema).MutateOperationContext(t.Context(), rc)
	}

	for name, test := range map[string]func(t *testing.T){
		"AllowsQueryUnderLimit": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline + 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))

			assert.Nil(t, run(t))
		},
		"RejectsQueryOverLimit": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline - 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))

			gqlErr := run(t)
			require.NotNil(t, gqlErr)
			assert.Equal(t, ComplexityLimitExceeded, gqlErr.Extensions["code"])
		},
		"WarnsButServesWhenLimiterDisabled": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline - 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))
			settings.ServiceFlags.GraphQLComplexityLimiterDisabled = true
			require.NoError(t, settings.ServiceFlags.Set(t.Context()))

			// Swap the grip sender to capture the warn-only log line.
			defer func(s send.Sender) {
				assert.NoError(t, grip.SetSender(s))
			}(grip.GetSender())
			sender := send.MakeInternalLogger()
			require.NoError(t, grip.SetSender(sender))

			assert.Nil(t, run(t))
			// Warn mode must emit a log line: it's the only signal that an
			// over-limit query was allowed through.
			assert.True(t, sender.HasMessage())
		},
		"ZeroLimitDisablesEnforcement": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = 0
			require.NoError(t, settings.RateLimit.Set(t.Context()))

			assert.Nil(t, run(t))
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
			test(t)
		})
	}
}
