package core

import (
	"bytes"
	"fmt"
	"go/importer"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"golang.org/x/tools/imports"
)

var pathPrefix = ""

const (
	fileTemplatePath           = "templates/file.gotmpl"
	structTemplatePath         = "templates/struct.gotmpl"
	fieldTemplatePath          = "templates/field.gotmpl"
	serviceMethodsTemplatePath = "templates/service_methods.gotmpl"
	arrayMethodsTemplatePath   = "templates/array_methods.gotmpl"
	bfsConvertTemplatePath     = "templates/buildfromservice_conversion.gotmpl"
	tsConvertTemplatePath      = "templates/toservice_conversion.gotmpl"
	funcTemplatePath           = "templates/func_call.gotmpl"
)

const (
	aliasesDirective = "aliases"
	dbFieldNameArg   = "db"
	jsonTagArg       = "json"
)

type fileInfo struct {
	Package string
	Structs string
	Code    string
}

type structInfo struct {
	Name   string
	Fields string
}

type extractedField struct {
	OutputFieldName string
	OutputFieldType string
	Nullable        bool
	JsonTag         string
}

type extractedFields map[string]extractedField

type conversionLine struct {
	RestField          string
	TypeConversionFunc string
	ModelField         string
}

type modelConversionInfo struct {
	RestType       string
	ModelType      string
	BfsConversions string
	TsConversions  string
}

type arrayConversionInfo struct {
	FromType       string
	FromHasPtr     bool
	ToType         string
	ToHasPtr       bool
	ConversionCode string
}

// ModelMapping maps schema type names to their respective DB model
type ModelMapping map[string]string

// SetGeneratePathPrefix allows callers outside this package to set the path to this package,
// since it uses relative paths for all the templates
func SetGeneratePathPrefix(prefix string) {
	pathPrefix = prefix
}

// Codegen takes a GraphQL schema as well as a mapping file of GQL structs to
// DB structs, then returns a generated REST model with conversion code, as well
// as utility code to convert between pointers
func Codegen(schema string, config ModelMapping) ([]byte, []byte, error) {
	source := ast.Source{
		Input: schema,
	}
	fileTemplate, err := getTemplate(fileTemplatePath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error loading file template")
	}
	structTemplate, err := getTemplate(structTemplatePath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error loading struct template")
	}
	fieldTemplate, err := getTemplate(fieldTemplatePath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error loading field template")
	}
	parsedAst, parseErr := gqlparser.LoadSchema(&source)
	if parseErr != nil {
		return nil, nil, errors.Wrap(parseErr, "error parsing schema")
	}
	catcher := grip.NewBasicCatcher()
	structs := ""
	serviceCode := ""
	generatedConversions := map[typeInfo]string{}

	typeNames := []string{}
	for key := range parsedAst.Types {
		typeNames = append(typeNames, key)
	}
	sort.Strings(typeNames)
	for _, typeName := range typeNames {
		dbModel, shouldConvert := config[typeName]
		if !shouldConvert {
			continue
		}
		gqlType := parsedAst.Types[typeName]
		if gqlType == nil {
			catcher.Errorf("unable to find type '%s'", typeName)
			continue
		}
		if gqlType.BuiltIn {
			continue
		}
		if gqlType.Kind == ast.Scalar {
			continue
		}

		var structData string
		var code string
		structData, code, err = generateForStruct(fieldTemplate, structTemplate, typeName, dbModel, *gqlType, generatedConversions)
		if err != nil {
			catcher.Add(err)
			continue
		}
		structs += structData
		serviceCode += code
	}
	modelFile, err := output(fileTemplate, fileInfo{Package: "model", Structs: structs, Code: serviceCode})
	catcher.Add(err)
	formattedModelCode, err := goimports(modelFile)
	catcher.Wrap(err, "unable to format model code")

	converters := typeInfoSorter{}
	conversionCode := ""
	for key := range generatedConversions {
		converters = append(converters, key)
	}
	sort.Sort(&converters)
	for _, converterType := range converters {
		conversionCode += generatedConversions[converterType] + "\n"
	}
	converterFile, err := output(fileTemplate, fileInfo{Package: "model", Code: conversionCode})
	catcher.Add(err)
	formattedConverterFile, err := goimports(converterFile)
	catcher.Wrap(err, "unable to format type converter code")

	return formattedModelCode, formattedConverterFile, catcher.Resolve()
}

// generateForStruct takes a specific db type and returns the code for the REST type
// as well as BuildFromService/ToService conversion methods
func generateForStruct(fieldTemplate, structTemplate *template.Template, typeName, dbModel string, gqlType ast.Definition, generatedConversions map[typeInfo]string) (string, string, error) {
	fields := ""
	extracted := extractedFields{}
	for _, field := range gqlType.Fields {
		fieldInfo := getFieldInfo(field)
		extracted[dbField(*field)] = fieldInfo
		fieldData, err := output(fieldTemplate, fieldInfo)
		if err != nil {
			return "", "", err
		}
		fields += fieldData
	}
	parts := strings.Split(dbModel, ".")
	if len(parts) < 2 {
		return "", "", errors.Errorf("invalid format for DB model: %s", dbModel)
	}
	dbPkg := strings.Join(parts[:len(parts)-1], ".")
	dbStructName := parts[len(parts)-1]
	structData, err := output(structTemplate, structInfo{Name: "API" + dbStructName, Fields: fields})
	if err != nil {
		return "", "", err
	}

	code, err := createConversionMethods(dbPkg, dbStructName, extracted, generatedConversions)
	if err != nil {
		return "", "", errors.Wrapf(err, "error generating conversion methods for type '%s'", typeName)
	}

	return structData, code, nil
}

// dbField looks for the "db" schema directive, which tells us what field in the db model
// matches up to the given field in the REST model, in case they are not named the same thing
func dbField(gqlDefinition ast.FieldDefinition) string {
	name := gqlDefinition.Name
	directive := gqlDefinition.Directives.ForName(aliasesDirective)
	if directive != nil {
		arg := directive.Arguments.ForName(dbFieldNameArg)
		if arg != nil {
			name = arg.Value.String()
		}
	}
	return name
}

// getTemplate is a utility function that takes a filepath (relative to the directory of this file)
// and returns a parsed go template from it
func getTemplate(file string) (*template.Template, error) {
	f, err := os.ReadFile(filepath.Join(pathPrefix, file))
	if err != nil {
		return nil, err
	}
	return template.New(file).Funcs(template.FuncMap{
		"shortenpackage": shortenPackage,
		"cleanName":      cleanName,
		"mustBePtr":      mustBePtr,
		"mustBeValue":    mustBeValue,
		"containsPtr":    containsPtr,
	}).Parse(string(f))
}

// jsonTag looks for the "json" schema directive, which tells us what json tag the rest model field
// should have, in case the user does not like the default json tag we generate
func jsonTag(gqlDefinition ast.FieldDefinition) string {
	directive := gqlDefinition.Directives.ForName(aliasesDirective)
	if directive != nil {
		arg := directive.Arguments.ForName(jsonTagArg)
		if arg != nil {
			return arg.Value.String()
		}
	}
	pieces := words(gqlDefinition.Name)
	return strings.Join(pieces, "_")
}

// words takes a camelCased or SentenceCased field name and separates what it thinks are the
// separate words in the name, determined by the casing
func words(in string) []string {
	out := []string{}
	re := regexp.MustCompile(`[A-Za-z][^A-Z]*`)
	submatchall := re.FindAllString(in, -1)
	for _, element := range submatchall {
		out = append(out, strings.ToLower(element))
	}
	return out
}

// output is a utility function to apply an abitrary struct to an arbitrary template and return
// its output as a string
func output(t *template.Template, data interface{}) (string, error) {
	if t == nil {
		return "", errors.New("cannot execute nil template")
	}
	w := bytes.NewBuffer(nil)
	err := t.Execute(w, data)
	return w.String(), err
}

// getFieldInfo is a utility function to translate the data structure used by the third-party
// AST parser into a structure that the rest of the code here can understand
func getFieldInfo(f *ast.FieldDefinition) extractedField {
	outputType := gqlTypeToGoType(f.Type.String(), !f.Type.NonNull)
	return extractedField{
		OutputFieldName: f.Name,
		OutputFieldType: outputType,
		Nullable:        strings.Contains(outputType, "*"),
		JsonTag:         jsonTag(*f),
	}
}

// gqlTypeToGoType is a very important function that will take a built-in GraphQL type and return
// the corresponding Go type. This is used to determine the type of the field in the REST model
func gqlTypeToGoType(gqlType string, nullable bool) string {
	gqlType = strings.Replace(gqlType, "!", "", -1)
	returntype := ""
	if nullable {
		returntype = "*"
	}
	switch gqlType {
	case "String":
		return returntype + "string"
	case "Int":
		return returntype + "int"
	case "Boolean":
		return returntype + "bool"
	case "Float":
		return returntype + "float64"
	// the next 3 are built-in scalars from https://gqlgen.com/reference/scalars/#built-in-helpers
	case "Time":
		return returntype + "time.Time"
	case "Map":
		return returntype + "map[string]interface{}"
	case "Any":
		return returntype + "interface{}"
	default:
		re := regexp.MustCompile(`^\[([0-z]+)\]$`)
		isArray := re.MatchString(gqlType)
		if isArray {
			matches := re.FindStringSubmatch(gqlType)
			if len(matches) != 2 {
				grip.Errorf("type '%s' may be a nested array, which is not supported", gqlType)
				return ""
			}
			elem := matches[1]
			return "[]" + gqlTypeToGoType(elem, nullable)
		}
		return "API" + gqlType
	}
}

// goimports is the final step to format the generated code with goimports.
func goimports(source string) ([]byte, error) {
	return imports.Process("", []byte(source), &imports.Options{
		AllErrors: true, Comments: true, TabIndent: true, TabWidth: 8,
	})
}

// createConversionMethods takes a given package+struct in this code base and a specification of what fields
// are of interest, and generates the methods to convert between the REST and DB types. It does not generate
// the REST struct itself
func createConversionMethods(packageName, structName string, fields extractedFields, generatedConversions map[typeInfo]string) (string, error) {
	pkg, err := importer.Default().Import(packageName)
	if err != nil {
		return "", errors.Wrapf(err, "unable to resolve package '%s'", packageName)
	}
	scope := pkg.Scope()
	if scope == nil {
		return "", errors.Errorf("unable to parse symbols in package '%s'", packageName)
	}
	obj := scope.Lookup(structName)
	if obj == nil {
		return "", errors.Errorf("struct '%s' not found in package '%s'", structName, packageName)
	}
	structVal, isStruct := obj.Type().Underlying().(*types.Struct)
	if !isStruct {
		return "", errors.Errorf("identifier '%s' exists in package '%s' but is not a struct", structName, packageName)
	}

	code, err := generateServiceConversions(structVal, packageName, structName, fields, generatedConversions)
	if err != nil {
		return "", err
	}

	return code, nil
}

// generateServiceConversions is a utility function to generate the conversion code for a given struct,
// assumed to be a DB model
func generateServiceConversions(structVal *types.Struct, packageName, structName string, fields extractedFields, generatedConversions map[typeInfo]string) (string, error) {
	serviceTemplate, err := getTemplate(serviceMethodsTemplatePath)
	if err != nil {
		return "", errors.Wrap(err, "error getting service methods template")
	}
	bfsConvertTemplate, err := getTemplate(bfsConvertTemplatePath)
	if err != nil {
		return "", errors.Wrap(err, "error getting BuildFromService conversion template")
	}
	tsConvertTemplate, err := getTemplate(tsConvertTemplatePath)
	if err != nil {
		return "", errors.Wrap(err, "error getting ToService conversion template")
	}
	bfsCode := []string{}
	tsCode := []string{}
	for i := 0; i < structVal.NumFields(); i++ {
		field := structVal.Field(i)
		fieldName := field.Name()
		if fieldInfo, shouldExtract := fields[fieldName]; shouldExtract {
			// generate the BuildFromService code
			if warning := validateFieldTypes(fieldName, field.Type().String(), fieldInfo.OutputFieldType); warning != "" {
				grip.Warning(warning)
			}
			opts := convertFnOpts{
				in:       field.Type(),
				outIsPtr: fieldInfo.Nullable,
				outType:  fieldInfo.OutputFieldType,
			}
			convertFuncs, err := conversionFn(opts, generatedConversions)
			if err != nil {
				return "", errors.Wrapf(err, "unable to find model conversion function for field %s", fieldName)
			}
			data := conversionLine{
				ModelField:         fieldName,
				RestField:          fieldInfo.OutputFieldName,
				TypeConversionFunc: convertFuncs.converter,
			}
			lineData, err := output(bfsConvertTemplate, data)
			if err != nil {
				return "", errors.Wrap(err, "error generating BuildFromService code")
			}
			bfsCode = append(bfsCode, lineData)

			// generate the ToService code
			data = conversionLine{
				ModelField:         fieldName,
				RestField:          fieldInfo.OutputFieldName,
				TypeConversionFunc: convertFuncs.inverter,
			}
			lineData, err = output(tsConvertTemplate, data)
			if err != nil {
				return "", errors.Wrap(err, "error generating ToService code")
			}
			tsCode = append(tsCode, lineData)
			_, isArray := field.Type().(*types.Array)
			_, isSlice := field.Type().(*types.Slice)
			if !isArray && !isSlice {
				continue
			}
			_, err = generateArrayConversions(fieldName, field.Type(), fieldInfo.OutputFieldName, fieldInfo.OutputFieldType, fieldInfo.Nullable, generatedConversions)
			if err != nil {
				return "", errors.Wrap(err, "error generating array conversion code")
			}
		}
		sort.Strings(bfsCode)
		sort.Strings(tsCode)
	}
	data := modelConversionInfo{
		ModelType:      fmt.Sprintf("%s.%s", packageName, structName),
		RestType:       fmt.Sprintf("API%s", structName),
		BfsConversions: strings.Join(bfsCode, "\n"),
		TsConversions:  strings.Join(tsCode, "\n"),
	}
	return output(serviceTemplate, data)
}

// findArrayElem is a utility function to return the type of element of an array,
// or nil if it is not an array
func findArrayElem(arrayType types.Type) *types.Type {
	switch v := arrayType.(type) {
	case *types.Slice:
		elem := v.Elem()
		// TODO: throwing away the pointer makes this unable to handle converting
		// a model type that is a slice of pointers to a rest type that is a slice
		// of non-pointers
		if value, isPtr := elem.(*types.Pointer); isPtr {
			underlying := value.Underlying()
			return &underlying
		}
		return &elem
	case *types.Array:
		elem := v.Elem()
		if value, isPtr := elem.(*types.Pointer); isPtr {
			underlying := value.Underlying()
			return &underlying
		}
		return &elem
	}
	return nil
}

// validateFieldTypes is currently just a method to compare the names of the types of two fields, and
// potentially generate a warning if the type names are not sufficiently close to each other. Should
// ideally be replaced with a more robust validation function
func validateFieldTypes(fieldName, inputType, outputType string) string {
	inputIsArray, inputType := stripArray(inputType)
	outputIsArray, outputType := stripArray(outputType)
	if inputIsArray != outputIsArray {
		return fmt.Sprintf("Only one of field '%s' or its counterpart is an array", fieldName)
	}
	inputType = stripPackage(inputType)
	outputType = stripPackage(outputType)
	if !strings.Contains(outputType, inputType) && !strings.Contains(inputType, outputType) {
		// this should ideally be a more sophisticated check to ensure that complex types are convertible
		// to each other, but for now we rely on the naming convention to find potential type errors
		return fmt.Sprintf("DB model field '%s' has type '%s' which may be incompatible with REST model type '%s'", fieldName, inputType, outputType)
	}

	return ""
}
