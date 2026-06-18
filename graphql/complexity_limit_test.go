package graphql

import (
	"context"
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

			defer func(s send.Sender) {
				assert.NoError(t, grip.SetSender(s))
			}(grip.GetSender())
			sender := send.MakeInternalLogger()
			require.NoError(t, grip.SetSender(sender))

			gqlErr := run(t)
			require.NotNil(t, gqlErr)
			assert.Equal(t, ComplexityLimitExceeded, gqlErr.Extensions["code"])
			assert.Equal(t, baseline, gqlErr.Extensions["complexity"])
			assert.Equal(t, baseline-1, gqlErr.Extensions["limit"])
			assert.True(t, sender.HasMessage(), "rejection should be logged")
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
		"ConfigReadErrorAllowsThrough": func(t *testing.T) {
			// Simulate a settings read failure by cancelling the context before
			// the DB call. The extension must not block the query in this case.
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			rc := &graphql.OperationContext{Operation: op}
			assert.Nil(t, MakeComplexityLimit(schema).MutateOperationContext(ctx, rc))
		},
		// The extension reads config fresh on every operation, so flipping the
		// limit or the disable flag changes the outcome for an identical query
		// with no process restart.
		"ZeroLimitUnblocksAfterRejection": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline - 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))
			require.NotNil(t, run(t))

			settings.RateLimit.GraphQLComplexityLimit = 0
			require.NoError(t, settings.RateLimit.Set(t.Context()))
			assert.Nil(t, run(t), "zeroing the limit should let the same query through")
		},
		"DisableFlagUnblocksAfterRejection": func(t *testing.T) {
			settings, err := evergreen.GetConfig(t.Context())
			require.NoError(t, err)
			settings.RateLimit.GraphQLComplexityLimit = baseline - 1
			require.NoError(t, settings.RateLimit.Set(t.Context()))
			require.NotNil(t, run(t))

			settings.ServiceFlags.GraphQLComplexityLimiterDisabled = true
			require.NoError(t, settings.ServiceFlags.Set(t.Context()))
			assert.Nil(t, run(t), "disabling the limiter should let the same query through")
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
			test(t)
		})
	}
}
