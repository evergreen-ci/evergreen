package graphql

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

// unboundedListSize is the list size assumed for a field whose request
// returns every matching item.
const unboundedListSize = 100

// listLimit describes how a list field's requested size is determined.
type listLimit struct {
	// arg is the dot-separated path to the limit argument, e.g. "opts.limit".
	arg string
	// fallback is the size the resolver uses when the limit is unset (and has
	// no schema default) or is not positive.
	fallback int
}

// listLimitFields are the fields whose complexity is multiplied by the number
// of items requested. A field belongs here only if its limit bounds the
// items the resolver fetches. For waterfall and mainlineCommits, the limit
// counts versions but each version's builds and tasks are unbounded, so their
// scores are lower bounds.
var listLimitFields = map[string]listLimit{
	"Host.events":           {arg: "opts.limit", fallback: unboundedListSize},
	"Image.events":          {arg: "limit", fallback: unboundedListSize},
	"Image.files":           {arg: "opts.limit", fallback: unboundedListSize},
	"Image.operatingSystem": {arg: "opts.limit", fallback: unboundedListSize},
	"Image.packages":        {arg: "opts.limit", fallback: unboundedListSize},
	"Image.toolchains":      {arg: "opts.limit", fallback: unboundedListSize},
	"Project.patches":       {arg: "patchesInput.limit", fallback: unboundedListSize},
	"Query.adminEvents":     {arg: "opts.limit", fallback: unboundedListSize},
	"Query.distroEvents":    {arg: "opts.limit", fallback: unboundedListSize},
	"Query.hosts":           {arg: "limit", fallback: unboundedListSize},
	"Query.mainlineCommits": {arg: "options.limit", fallback: 7},
	"Query.projectEvents":   {arg: "limit", fallback: 10},
	"Query.repoEvents":      {arg: "limit", fallback: 10},
	"Query.taskHistory":     {arg: "options.limit", fallback: 50},
	"Query.waterfall":       {arg: "options.limit", fallback: 5},
	"Task.tests":            {arg: "opts.limit", fallback: unboundedListSize},
	"User.patches":          {arg: "patchesInput.limit", fallback: unboundedListSize},
	"Version.tasks":         {arg: "options.limit", fallback: unboundedListSize},
}

// NewSchema returns the executable GraphQL schema with complexity scoring
// that multiplies list fields by the number of items requested.
func NewSchema(apiURL string) graphql.ExecutableSchema {
	es := NewExecutableSchema(New(apiURL))
	defaults, err := listLimitDefaults(es.Schema(), listLimitFields)
	if err != nil {
		panic(err)
	}
	return complexitySchema{ExecutableSchema: es, limitDefaults: defaults}
}

// complexitySchema overrides the complexity of list fields. It reads limits
// directly from the raw arguments rather than using gqlgen's Config complexity
// functions, because the generated argument unmarshalling runs input
// directives such as @requireProjectAccess, which query the database and
// silently fall back to the default complexity when they fail.
type complexitySchema struct {
	graphql.ExecutableSchema
	// limitDefaults maps each field in listLimitFields to the schema default
	// of its limit argument, if it has one.
	limitDefaults map[string]*int
}

func (s complexitySchema) Complexity(ctx context.Context, typeName, fieldName string, childComplexity int, args map[string]any) (int, bool) {
	field := typeName + "." + fieldName
	spec, ok := listLimitFields[field]
	if !ok {
		return s.ExecutableSchema.Complexity(ctx, typeName, fieldName, childComplexity, args)
	}
	size := spec.fallback
	if limit, ok := argInt(args, spec.arg); ok {
		if limit > 0 {
			size = limit
		}
	} else if def := s.limitDefaults[field]; def != nil && *def > 0 {
		size = *def
	}
	return saturatingAdd(1, saturatingMultiply(childComplexity, size)), true
}

// argInt returns the integer at the dot-separated path in args, or false if
// it is unset or not an integer.
func argInt(args map[string]any, path string) (int, bool) {
	keys := strings.Split(path, ".")
	for _, key := range keys[:len(keys)-1] {
		nested, ok := args[key].(map[string]any)
		if !ok {
			return 0, false
		}
		args = nested
	}
	raw, ok := args[keys[len(keys)-1]]
	if !ok || raw == nil {
		return 0, false
	}
	v, err := graphql.UnmarshalInt(raw)
	return v, err == nil
}

// listLimitDefaults resolves each field's limit argument against the schema
// and returns its default value. It errors if a field or argument path does
// not exist so that a stale entry fails at startup.
func listLimitDefaults(schema *ast.Schema, fields map[string]listLimit) (map[string]*int, error) {
	defaults := map[string]*int{}
	for field, spec := range fields {
		typeName, fieldName, _ := strings.Cut(field, ".")
		def := schema.Types[typeName]
		if def == nil || def.Fields.ForName(fieldName) == nil {
			return nil, fmt.Errorf("list limit field '%s' not found in schema", field)
		}
		keys := strings.Split(spec.arg, ".")
		arg := def.Fields.ForName(fieldName).Arguments.ForName(keys[0])
		if arg == nil {
			return nil, fmt.Errorf("argument '%s' not found on field '%s'", keys[0], field)
		}
		argType, defaultValue := arg.Type, arg.DefaultValue
		for _, key := range keys[1:] {
			input := schema.Types[argType.Name()]
			if input == nil || input.Kind != ast.InputObject || input.Fields.ForName(key) == nil {
				return nil, fmt.Errorf("argument path '%s' not found on field '%s'", spec.arg, field)
			}
			argType, defaultValue = input.Fields.ForName(key).Type, input.Fields.ForName(key).DefaultValue
		}
		if argType.Name() != "Int" {
			return nil, fmt.Errorf("argument '%s' on field '%s' must be an Int", spec.arg, field)
		}
		if defaultValue != nil {
			v, err := defaultValue.Value(nil)
			if err != nil {
				return nil, fmt.Errorf("reading default for argument '%s' on field '%s': %w", spec.arg, field, err)
			}
			i, err := graphql.UnmarshalInt(v)
			if err != nil {
				return nil, fmt.Errorf("reading default for argument '%s' on field '%s': %w", spec.arg, field, err)
			}
			defaults[field] = &i
		}
	}
	return defaults, nil
}

// saturatingMultiply returns a*b for non-negative operands, capped at
// math.MaxInt so that a very large requested limit cannot overflow into a
// small or negative score.
func saturatingMultiply(a, b int) int {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > math.MaxInt/b {
		return math.MaxInt
	}
	return a * b
}

// saturatingAdd returns a+b for non-negative operands, capped at math.MaxInt.
func saturatingAdd(a, b int) int {
	if a > math.MaxInt-b {
		return math.MaxInt
	}
	return a + b
}
