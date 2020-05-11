package model

import (
	"bytes"
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
	fileTemplate   = "templates/file.gotmpl"
	structTemplate = "templates/struct.gotmpl"
	fieldTemplate  = "templates/field.gotmpl"
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

type fieldInfo struct {
	Name    string
	Type    string
	JsonTag string
}

func SchemaToGo(schema string) ([]byte, error) {
	source := ast.Source{
		Input: schema,
	}
	fileTemplate, err := getFileTmpl()
	if err != nil {
		return nil, errors.Wrap(err, "error loading file template")
	}
	structTemplate, err := getStructTmpl()
	if err != nil {
		return nil, errors.Wrap(err, "error loading struct template")
	}
	fieldTemplate, err := getFieldTmpl()
	if err != nil {
		return nil, errors.Wrap(err, "error loading field template")
	}
	parsedAst, parseErr := gqlparser.LoadSchema(&source)
	if parseErr != nil {
		return nil, errors.Wrap(parseErr, "error parsing schema")
	}
	catcher := grip.NewBasicCatcher()
	structs := ""

	typeNames := []string{}
	for key := range parsedAst.Types {
		typeNames = append(typeNames, key)
	}
	sort.Strings(typeNames)
	for _, typeName := range typeNames {
		gqlType := parsedAst.Types[typeName]
		if gqlType.BuiltIn {
			continue
		}
		if gqlType.Kind == ast.Scalar {
			continue
		}
		fields := ""
		for _, field := range gqlType.Fields {
			fieldData, err := output(fieldTemplate, getFieldInfo(field))
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
	}
	file, err := output(fileTemplate, fileInfo{Package: "model", Structs: structs})
	catcher.Add(err)
	formatted, err := goimports(file)
	catcher.Add(err)

	return formatted, catcher.Resolve()
}

func getStructTmpl() (*template.Template, error) {
	f, err := ioutil.ReadFile(structTemplate)
	if err != nil {
		return nil, err
	}
	return template.New("struct").Parse(string(f))
}

func getFileTmpl() (*template.Template, error) {
	f, err := ioutil.ReadFile(fileTemplate)
	if err != nil {
		return nil, err
	}
	return template.New("file").Parse(string(f))
}

func getFieldTmpl() (*template.Template, error) {
	f, err := ioutil.ReadFile(fieldTemplate)
	if err != nil {
		return nil, err
	}
	return template.New("field").Parse(string(f))
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

func getFieldInfo(f *ast.FieldDefinition) fieldInfo {
	name, tag := nameAndTag(f.Name)
	return fieldInfo{
		Name:    name,
		Type:    gqlTypeToGoType(f.Type.Name()),
		JsonTag: tag,
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
