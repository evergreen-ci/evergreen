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

// conversionFn uses the specific naming conventions of type conversion funcs in conversions.gotmpl
// to find the correct generated type conversion function
func conversionFn(in types.Type, outIsPtr bool, generatedConversions map[string]string) (string, error) {
	if _, inIsPrimitive := in.(*types.Basic); inIsPrimitive {
		return noTransforms(in.String(), outIsPtr, generatedConversions)
	}
	if intype, inIsPtr := in.(*types.Pointer); inIsPtr {
		value, isPrimitive := intype.Elem().(*types.Basic)
		if isPrimitive {
			return noTransforms(value.String(), outIsPtr, generatedConversions)
		}
		return conversionFn(value, outIsPtr, generatedConversions)
	}
	if intype, inIsNamedType := in.(*types.Named); inIsNamedType {
		switch intype.String() {
		case "time.Time":
			return noTransforms(intype.String(), outIsPtr, generatedConversions)
		default:
			return "", errors.New("complex objects not implemented yet")
		}
	}
	if intype, inIsMap := in.(*types.Map); inIsMap {
		switch intype.String() {
		case "map[string]interface{}":
			return noTransforms(intype.String(), outIsPtr, generatedConversions)
		default:
			return "", errors.New("complex maps not implemented yet")
		}
	}
	if intype, inIsInterface := in.(*types.Interface); inIsInterface {
		switch intype.String() {
		case "interface{}":
			return noTransforms(intype.String(), outIsPtr, generatedConversions)
		default:
			return "", errors.New("non-empty interfaces not implemented yet")
		}
	}

	return "", errors.Errorf("converting type %s is not supported", in.String())
}

func noTransforms(typeName string, outIsPtr bool, generatedConversions map[string]string) (string, error) {
	if err := generateConversionCode(generatedConversions, typeName); err != nil {
		return "", err
	}
	typeName = cleanName(typeName)
	if outIsPtr {
		return fmt.Sprintf("%s%sPtr", typeName, typeName), nil
	}
	return fmt.Sprintf("%s%s", typeName, typeName), nil
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
