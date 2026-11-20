package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

// DetectDeprecatedFields surfaces deprecated fields in "extensions" in the GraphQL response.
type DetectDeprecatedFields struct {
	schema *ast.Schema
}

type deprecatedField struct {
	Field  string `json:"field"`
	Reason string `json:"reason,omitempty"`
}

func (*DetectDeprecatedFields) ExtensionName() string {
	return "DetectDeprecatedFields"
}

func (d *DetectDeprecatedFields) Validate(schema graphql.ExecutableSchema) error {
	d.schema = schema.Schema()
	return nil
}

func (d *DetectDeprecatedFields) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	if !graphql.HasOperationContext(ctx) {
		return next(ctx)
	}

	rc := graphql.GetOperationContext(ctx)
	if rc.Operation == nil {
		return next(ctx)
	}

	deprecationVisitor := &deprecationVisitor{
		schema:    d.schema,
		variables: rc.Variables,
		found:     make(map[string]string),
		fragments: make(map[string]bool),
	}
	deprecationVisitor.walkSelections(rc.Operation.SelectionSet)

	deprecated := make([]deprecatedField, 0, len(deprecationVisitor.found))
	for field, reason := range deprecationVisitor.found {
		deprecated = append(deprecated, deprecatedField{Field: field, Reason: reason})
	}
	if len(deprecated) > 0 {
		graphql.RegisterExtension(ctx, "deprecatedFields", deprecated)
	}
	return next(ctx)
}

type deprecationVisitor struct {
	schema    *ast.Schema
	variables map[string]any
	found     map[string]string
	fragments map[string]bool
}

func (v *deprecationVisitor) recordIfDeprecated(pathToField string, directives ast.DirectiveList) {
	directive := directives.ForName("deprecated")
	if directive == nil {
		return
	}
	reason, ok := directive.ArgumentMap(nil)["reason"].(string)
	if !ok {
		reason = "this field is no longer supported"
	}
	v.found[pathToField] = reason
}

func (v *deprecationVisitor) walkSelections(sels ast.SelectionSet) {
	for _, sel := range sels {
		switch s := sel.(type) {
		case *ast.InlineFragment:
			v.walkSelections(s.SelectionSet)
		case *ast.FragmentSpread:
			if s.Definition == nil || v.fragments[s.Name] {
				continue
			}
			v.fragments[s.Name] = true
			v.walkSelections(s.Definition.SelectionSet)
		// Base case that covers top-level queries, mutations, and fields.
		case *ast.Field:
			if s.Definition == nil || s.ObjectDefinition == nil {
				return
			}
			v.recordIfDeprecated(fmt.Sprintf("%s.%s", s.ObjectDefinition.Name, s.Name), s.Definition.Directives)
			v.walkFieldArguments(s)
			v.walkSelections(s.SelectionSet)
		}
	}
}

func (v *deprecationVisitor) walkFieldArguments(field *ast.Field) {
	for _, arg := range field.Arguments {
		argDef := field.Definition.Arguments.ForName(arg.Name)
		if argDef == nil {
			continue
		}
		v.recordIfDeprecated(fmt.Sprintf("%s.%s.%s", field.ObjectDefinition.Name, field.Name, arg.Name), argDef.Directives)
		value, err := arg.Value.Value(v.variables)
		if err == nil {
			v.walkInputValue(argDef.Type, value)
		}
	}
}

func (v *deprecationVisitor) walkInputValue(expectedType *ast.Type, value any) {
	switch typedValue := value.(type) {
	case map[string]any:
		inputType := v.schema.Types[expectedType.Name()]
		if inputType == nil {
			return
		}
		for fieldName, fieldValue := range typedValue {
			fieldDeclaration := inputType.Fields.ForName(fieldName)
			if fieldDeclaration == nil {
				continue
			}
			v.recordIfDeprecated(fmt.Sprintf("%s.%s", inputType.Name, fieldName), fieldDeclaration.Directives)
			v.walkInputValue(fieldDeclaration.Type, fieldValue)
		}
	case []any:
		for _, item := range typedValue {
			v.walkInputValue(expectedType, item)
		}
	}
}
