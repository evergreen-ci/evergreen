package model

import (
	"fmt"
	"go/types"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

const (
	conversionTemplatePath = "templates/conversions.gotmpl"
)

type typeInfo struct {
	Type string
}

const (
	buildFromService = "BuildFromService"
	toService        = "ToService"
)

// conversionFn uses the specific naming conventions of type conversion funcs in conversions.gotmpl
// to find the correct generated type conversion function. It returns the name of the conversion function,
// then the name of the inverse conversion function
func conversionFn(in types.Type, outIsPtr bool, generatedConversions map[string]string) (string, string, error) {
	if _, inIsPrimitive := in.(*types.Basic); inIsPrimitive {
		return simplePtrs(in.String(), outIsPtr, generatedConversions)
	}
	if intype, inIsPtr := in.(*types.Pointer); inIsPtr {
		value, isPrimitive := intype.Elem().(*types.Basic)
		if isPrimitive {
			return simplePtrs(value.String(), outIsPtr, generatedConversions)
		}
		underlyingStruct, isStruct := intype.Elem().(*types.Named)
		if isStruct {
			return objectTypeFnName(underlyingStruct, outIsPtr)
		}
		return "", "", errors.New("complex pointers not implemented yet")
	}
	if intype, inIsNamedType := in.(*types.Named); inIsNamedType {
		switch intype.String() {
		case "time.Time":
			return simplePtrs(intype.String(), outIsPtr, generatedConversions)
		default:
			return objectTypeFnName(intype, outIsPtr)
		}
	}
	if intype, inIsMap := in.(*types.Map); inIsMap {
		switch intype.String() {
		case "map[string]interface{}":
			return simplePtrs(intype.String(), outIsPtr, generatedConversions)
		default:
			return "", "", errors.New("complex maps not implemented yet")
		}
	}
	if intype, inIsInterface := in.(*types.Interface); inIsInterface {
		switch intype.String() {
		case "interface{}":
			return simplePtrs(intype.String(), outIsPtr, generatedConversions)
		default:
			return "", "", errors.New("non-empty interfaces not implemented yet")
		}
	}

	return "", "", errors.Errorf("converting type %s is not supported", in.String())
}

func objectTypeFnName(in types.Type, outIsPtr bool) (string, string, error) {
	fnName := ""
	if !outIsPtr {
		fnName = "*"
	}
	fnName += "API"
	fnName += stripPackage(in.String())
	return fnName + buildFromService, fnName + toService, nil
}

func simplePtrs(typeName string, outIsPtr bool, generatedConversions map[string]string) (string, string, error) {
	if err := generateConversionCode(generatedConversions, typeName); err != nil {
		return "", "", err
	}
	typeName = cleanName(typeName)
	if outIsPtr {
		return fmt.Sprintf("%s%sPtr", typeName, typeName), fmt.Sprintf("%sPtr%s", typeName, typeName), nil
	}
	out := fmt.Sprintf("%s%s", typeName, typeName)
	return out, out, nil
}

func cleanName(name string) string {
	name = strings.Replace(name, "[]", "Arr", -1)
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	return strings.Title(regex.ReplaceAllString(name, ""))
}

func generateConversionCode(generatedConversions map[string]string, typeName string) error {
	if generatedConversions[typeName] != "" {
		return nil
	}
	template, err := getTemplate(conversionTemplatePath)
	if err != nil {
		return errors.Wrap(err, "error loading conversion template")
	}
	data := typeInfo{
		Type: typeName,
	}
	output, err := output(template, data)
	if err != nil {
		return errors.Wrap(err, "error generating conversion code")
	}
	generatedConversions[typeName] = output
	return nil
}

func shortenPackage(pkg string) string {
	split := strings.Split(pkg, "/")
	return split[len(split)-1]
}

func stripPackage(pkg string) string {
	split := strings.Split(pkg, ".")
	return split[len(split)-1]
}
