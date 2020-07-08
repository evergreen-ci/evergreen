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
	InType  string
	OutType string
}

type typeInfoSorter []typeInfo

const (
	buildFromService = "BuildFromService"
	toService        = "ToService"
)

type convertFuncs struct {
	converter string
	inverter  string
}

type convertFnOpts struct {
	in       types.Type
	outIsPtr bool
	outType  string
}

// conversionFn uses the specific naming conventions of type conversion funcs in conversions.gotmpl
// to find the correct generated type conversion function. It returns the name of the conversion function
// and the name of the inverse conversion function
func conversionFn(opts convertFnOpts, generatedConversions map[typeInfo]string) (convertFuncs, error) {
	if _, inIsPrimitive := opts.in.(*types.Basic); inIsPrimitive {
		return simplePtrs(opts.in.String(), opts.outType, false, opts.outIsPtr, generatedConversions)
	}
	if intype, inIsPtr := opts.in.(*types.Pointer); inIsPtr {
		value, isPrimitive := intype.Elem().(*types.Basic)
		if isPrimitive {
			return simplePtrs(value.String(), opts.outType, true, opts.outIsPtr, generatedConversions)
		}
		underlyingStruct, isStruct := intype.Elem().(*types.Named)
		if isStruct {
			return objectTypeFnName(underlyingStruct, opts.outIsPtr)
		}
		return convertFuncs{}, errors.New("complex pointers not implemented yet")
	}
	if intype, inIsNamedType := opts.in.(*types.Named); inIsNamedType {
		switch intype.String() {
		case "time.Time":
			return simplePtrs(intype.String(), opts.outType, false, opts.outIsPtr, generatedConversions)
		default:
			return objectTypeFnName(intype, opts.outIsPtr)
		}
	}
	if intype, inIsMap := opts.in.(*types.Map); inIsMap {
		switch intype.String() {
		case "map[string]interface{}":
			return simplePtrs(intype.String(), opts.outType, false, opts.outIsPtr, generatedConversions)
		default:
			return convertFuncs{}, errors.New("complex maps not implemented yet")
		}
	}
	if intype, inIsInterface := opts.in.(*types.Interface); inIsInterface {
		switch intype.String() {
		case "interface{}":
			return simplePtrs(intype.String(), opts.outType, false, opts.outIsPtr, generatedConversions)
		default:
			return convertFuncs{}, errors.New("non-empty interfaces not implemented yet")
		}
	}

	return convertFuncs{}, errors.Errorf("converting type %s is not supported", opts.in.String())
}

func objectTypeFnName(in types.Type, outIsPtr bool) (convertFuncs, error) {
	funcs := convertFuncs{}
	fnName := ""
	if !outIsPtr {
		fnName = "*"
	}
	fnName += "API"
	fnName += stripPackage(in.String())
	funcs.converter = fnName + buildFromService
	funcs.inverter = fnName + toService
	return funcs, nil
}

func simplePtrs(inType, outType string, inIsPtr, outIsPtr bool, generatedConversions map[typeInfo]string) (convertFuncs, error) {
	funcs := convertFuncs{}
	if err := generateConversionCode(generatedConversions, inType, outType); err != nil {
		return funcs, err
	}
	if inType != outType {
		if err := generateConversionCode(generatedConversions, outType, inType); err != nil {
			return funcs, err
		}
	}
	inType = cleanName(inType, inIsPtr)
	outType = cleanName(outType, outIsPtr)
	funcs.converter = fmt.Sprintf("%s%s", inType, outType)
	funcs.inverter = fmt.Sprintf("%s%s", outType, inType)
	return funcs, nil
}

func cleanName(name string, addPtr bool) string {
	name = strings.Replace(name, "[]", "Arr", -1)
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	name = strings.Title(regex.ReplaceAllString(name, ""))
	if addPtr {
		name += "Ptr"
	}
	return name
}

func generateConversionCode(generatedConversions map[typeInfo]string, inType, outType string) error {
	data := typeInfo{
		InType:  mustBeValue(inType),
		OutType: mustBeValue(outType),
	}
	if generatedConversions[data] != "" {
		return nil
	}
	template, err := getTemplate(conversionTemplatePath)
	if err != nil {
		return errors.Wrap(err, "error loading conversion template")
	}
	output, err := output(template, data)
	if err != nil {
		return errors.Wrap(err, "error generating conversion code")
	}
	generatedConversions[data] = output
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

func mustBeValue(name string) string {
	return strings.Replace(name, "*", "", -1)
}

func mustBePtr(name string) string {
	return "*" + mustBeValue(name)
}

func (s *typeInfoSorter) Len() int {
	return len(*s)
}

func (s *typeInfoSorter) Swap(i, j int) {
	temp := (*s)[i]
	(*s)[i] = (*s)[j]
	(*s)[j] = temp
}

func (s *typeInfoSorter) Less(i, j int) bool {
	t := *s
	if t[i].InType == t[j].InType {
		return t[i].OutType < t[j].OutType
	}
	return t[i].InType < t[j].InType
}
