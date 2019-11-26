package apm

import "context"

type contextKey int

const (
	tagsContextKey contextKey = iota
)

func SetTags(ctx context.Context, tags ...string) context.Context {
	return context.WithValue(ctx, tagsContextKey, tags)
}

func GetTags(ctx context.Context) []string {
	val := ctx.Value(tagsContextKey)
	tags, ok := val.([]string)
	if !ok {
		return nil
	}
	return tags
}
