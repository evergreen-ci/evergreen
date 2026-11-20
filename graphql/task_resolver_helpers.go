package graphql

import (
	"context"

	gqlgen "github.com/99designs/gqlgen/graphql"
)

func shouldDecorateTestQuarantineStatus(ctx context.Context) bool {
	if !gqlgen.HasOperationContext(ctx) {
		return true
	}
	opCtx := gqlgen.GetOperationContext(ctx)
	fields := gqlgen.CollectFieldsCtx(ctx, nil)
	return collectedFieldPathContains(opCtx, fields, []string{"testResults", "isManuallyQuarantined"}) ||
		collectedFieldPathContains(opCtx, fields, []string{"testResults", "canQuarantine"})
}

func collectedFieldPathContains(opCtx *gqlgen.OperationContext, fields []gqlgen.CollectedField, path []string) bool {
	if len(path) == 0 {
		return true
	}
	for _, field := range fields {
		if field.Name != path[0] {
			continue
		}
		if len(path) == 1 {
			return true
		}
		if collectedFieldPathContains(opCtx, gqlgen.CollectFields(opCtx, field.Selections, nil), path[1:]) {
			return true
		}
	}
	return false
}
