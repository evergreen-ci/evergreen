package model

import (
	"bytes"
	"fmt"
	"go/importer"
	"go/types"
	"io/ioutil"
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

const (
	fileTemplatePath           = "templates/file.gotmpl"
	structTemplatePath         = "templates/struct.gotmpl"
	fieldTemplatePath          = "templates/field.gotmpl"
	serviceMethodsTemplatePath = "templates/service_methods.gotmpl"
	bfsConvertTemplatePath     = "templates/buildfromservice_conversion.gotmpl"
	tsConvertTemplatePath      = "templates/toservice_conversion.gotmpl"
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

// ModelMapping maps schema type names to their respective DB model
type ModelMapping map[string]string

func Codegen(schema string, config ModelMapping) ([]byte, error) {
	source := ast.Source{
		Input: schema,
	}
	fileTemplate, err := getTemplate(fileTemplatePath)
	if err != nil {
		return nil, errors.Wrap(err, "error loading file template")
	}
	structTemplate, err := getTemplate(structTemplatePath)
	if err != nil {
		return nil, errors.Wrap(err, "error loading struct template")
	}
	fieldTemplate, err := getTemplate(fieldTemplatePath)
	if err != nil {
		return nil, errors.Wrap(err, "error loading field template")
	}
	parsedAst, parseErr := gqlparser.LoadSchema(&source)
	if parseErr != nil {
		return nil, errors.Wrap(parseErr, "error parsing schema")
	}
	catcher := grip.NewBasicCatcher()
	structs := ""
	conversionCode := ""

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
		if gqlType.BuiltIn {
			continue
		}
		if gqlType.Kind == ast.Scalar {
			continue
		}
		fields := ""
		extractedFields := extractedFields{}
		for _, field := range gqlType.Fields {
			fieldInfo := getFieldInfo(field)
			extractedFields[field.Name] = fieldInfo
			fieldData, err := output(fieldTemplate, fieldInfo)
			if err != nil {
				catcher.Add(err)
				continue
			}
			fields += fieldData
		}
		structData, err := output(structTemplate, structInfo{Name: typeName, Fields: fields})
		if err != nil {
			catcher.Add(err)
			continue
		}
		structs += structData

		parts := strings.Split(dbModel, ".")
		if len(parts) < 2 {
			return nil, errors.Errorf("invalid format for DB model: %s", dbModel)
		}
		code, err := createConversionMethods(strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1], extractedFields)
		if err != nil {
			return nil, errors.Wrapf(err, "error generating conversion methods for type '%s'", typeName)
		}
		conversionCode += code
	}
	file, err := output(fileTemplate, fileInfo{Package: "model", Structs: structs, Code: conversionCode})
	catcher.Add(err)
	formatted, err := goimports(file)
	catcher.Add(err)

	return formatted, catcher.Resolve()
}

func getTemplate(filepath string) (*template.Template, error) {
	f, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return template.New(filepath).Funcs(template.FuncMap{
		"shortenpackage": func(pkg string) string {
			split := strings.Split(pkg, "/")
			return split[len(split)-1]
		},
	}).Parse(string(f))
}

func nameAndTag(fieldName string) (string, string) {
	pieces := words(fieldName)
	name := ""
	for _, piece := range pieces {
		name += strings.Title(piece)
	}
	return name, strings.Join(pieces, "_")
}
func words(in string) []string {
	out := []string{}
	re := regexp.MustCompile(`[A-Za-z][^A-Z]*`)
	submatchall := re.FindAllString(in, -1)
	for _, element := range submatchall {
		out = append(out, strings.ToLower(element))
	}
	return out
}

func output(t *template.Template, data interface{}) (string, error) {
	if t == nil {
		return "", errors.New("cannot execute nil template")
	}
	w := bytes.NewBuffer(nil)
	err := t.Execute(w, data)
	return string(w.Bytes()), err
}

func getFieldInfo(f *ast.FieldDefinition) extractedField {
	name, tag := nameAndTag(f.Name)
	outputType := gqlTypeToGoType(f.Type.Name())
	return extractedField{
		OutputFieldName: name,
		OutputFieldType: outputType,
		Nullable:        strings.Contains(outputType, "*"),
		JsonTag:         tag,
	}
}

func gqlTypeToGoType(gqlType string) string {
	switch gqlType {
	case "String":
		return "string"
	case "Int":
		return "int"
	case "Time":
		return "time.Time"
	default:
		return gqlType
	}
}

func goimports(source string) ([]byte, error) {
	return imports.Process("", []byte(source), &imports.Options{
		AllErrors: true, Comments: true, TabIndent: true, TabWidth: 8,
	})
}

func createConversionMethods(packageName, structName string, fields extractedFields) (string, error) {
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

	code, err := generateServiceConversions(structVal, packageName, structName, fields)
	if err != nil {
		return "", err
	}

	return code, nil
}

func generateServiceConversions(structVal *types.Struct, packageName, structName string, fields extractedFields) (string, error) {
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
	fieldErrs := grip.NewBasicCatcher()
	for i := 0; i < structVal.NumFields(); i++ {
		field := structVal.Field(i)
		fieldName := field.Name()
		if fieldInfo, shouldExtract := fields[fieldName]; shouldExtract {
			// generate the BuildFromService code
			if err = validateFieldTypes(fieldName, field.Type().String(), fieldInfo.OutputFieldType); err != nil {
				fieldErrs.Add(err)
				continue
			}
			converter, err := conversionFn(field.Type(), fieldInfo.Nullable)
			if err != nil {
				return "", errors.Wrapf(err, "unable to find model conversion function for field %s", fieldName)
			}
			data := conversionLine{
				ModelField:         fieldName,
				RestField:          fieldInfo.OutputFieldName,
				TypeConversionFunc: converter,
			}
			lineData, err := output(bfsConvertTemplate, data)
			if err != nil {
				return "", errors.Wrap(err, "error generating BuildFromService code")
			}
			bfsCode = append(bfsCode, lineData)

			// generate the ToService code
			converter, err = conversionFn(field.Type(), fieldInfo.Nullable) // TODO: this is wrong, figure out how to reverse
			if err != nil {
				return "", errors.Wrapf(err, "unable to find model conversion function for field %s", fieldName)
			}
			data = conversionLine{
				ModelField:         fieldName,
				RestField:          fieldInfo.OutputFieldName,
				TypeConversionFunc: converter,
			}
			lineData, err = output(tsConvertTemplate, data)
			if err != nil {
				return "", errors.Wrap(err, "error generating ToService code")
			}
			tsCode = append(tsCode, lineData)
		}
		sort.Strings(bfsCode)
		sort.Strings(tsCode)
	}
	if fieldErrs.HasErrors() {
		return "", fieldErrs.Resolve()
	}
	data := modelConversionInfo{
		ModelType:      fmt.Sprintf("%s.%s", packageName, structName),
		RestType:       fmt.Sprintf("API%s", structName),
		BfsConversions: strings.Join(bfsCode, "\n"),
		TsConversions:  strings.Join(tsCode, "\n"),
	}
	return output(serviceTemplate, data)
}

func validateFieldTypes(fieldName, inputType, outputType string) error {
	if !strings.Contains(outputType, inputType) && !strings.Contains(inputType, outputType) {
		// this should ideally be a more sophisticated check to ensure that complex types are convertible
		// to each other, but for now we rely on the naming convention to find obvious type errors
		return errors.Errorf("DB model field '%s' has type '%s' which is incompatible with REST model type '%s'", fieldName, inputType, outputType)
	}

	return nil
}
