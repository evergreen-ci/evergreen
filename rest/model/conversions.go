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
	InType   string
	InIsPtr  bool
	OutType  string
	OutIsPtr bool
}

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
	info := typeInfo{
		InType:   opts.in.String(),
		InIsPtr:  false,
		OutType:  opts.outType,
		OutIsPtr: opts.outIsPtr,
	}
	if _, inIsPrimitive := opts.in.(*types.Basic); inIsPrimitive {
		info.InType = opts.in.String()
		return simplePtrs(info, generatedConversions)
	}
	if intype, inIsPtr := opts.in.(*types.Pointer); inIsPtr {
		value, isPrimitive := intype.Elem().(*types.Basic)
		if isPrimitive {
			info.InType = value.String()
			info.InIsPtr = true
			return simplePtrs(info, generatedConversions)
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
			return simplePtrs(info, generatedConversions)
		default:
			return objectTypeFnName(intype, opts.outIsPtr)
		}
	}
	if intype, inIsMap := opts.in.(*types.Map); inIsMap {
		switch intype.String() {
		case "map[string]interface{}":
			return simplePtrs(info, generatedConversions)
		default:
			return convertFuncs{}, errors.New("complex maps not implemented yet")
		}
	}
	if intype, inIsInterface := opts.in.(*types.Interface); inIsInterface {
		switch intype.String() {
		case "interface{}":
			return simplePtrs(info, generatedConversions)
		default:
			return convertFuncs{}, errors.New("non-empty interfaces not implemented yet")
		}
	}

	return convertFuncs{}, errors.Errorf("converting type %s is not supported", opts.in.String())
}

// objectTypeFnName takes a data type and returns the names of the functions
// used to convert to/from that type in a way that is callable directly in code,
// using the very specific naming conventions defined in this file
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

// simplePtrs takes one set of desired input/output primitive data types and returns
// the names of the functions to convert between those data types in a way that is
// callable directly in code, using the naming conventions in this file
func simplePtrs(info typeInfo, generatedConversions map[typeInfo]string) (convertFuncs, error) {
	funcs := convertFuncs{}
	if err := generateConversionCode(generatedConversions, info.InType, info.OutType); err != nil {
		return funcs, err
	}
	if info.InType != info.OutType {
		if err := generateConversionCode(generatedConversions, info.OutType, info.InType); err != nil {
			return funcs, err
		}
	}
	inType := cleanName(info.InType, info.InIsPtr)
	outType := cleanName(info.OutType, info.OutIsPtr)
	funcs.converter = fmt.Sprintf("%s%s", inType, outType)
	funcs.inverter = fmt.Sprintf("%s%s", outType, inType)
	return funcs, nil
}

// cleanName takes the string representation of a go data type and replaces non-alphanumeric
// characters so that it can be used as part of a go function name
func cleanName(name string, addPtr bool) string {
	name = strings.Replace(name, "[]", "Arr", -1)
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	name = strings.Title(regex.ReplaceAllString(name, ""))
	if addPtr {
		name += "Ptr"
	}
	return name
}

// generateConversionCode generates the code to convert between two primitive data types,
// mostly to handle types that are directly castable to each other but have different names,
// or ones that just differ by a pointer. It maintains a map of what it has already generated
// so that callers do not need to worry about 2 fields having the same data type
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

// shortenPackage takes a fully qualified package+struct and shortens it to
// how it would be referenced in code
// for example: github.com/a/b/c.myStruct becomes c.myStruct
func shortenPackage(pkg string) string {
	split := strings.Split(pkg, "/")
	return split[len(split)-1]
}

// shortenPackage takes a package+struct name and strips the package
// for example: myPackage.myStruct becomes myStruct
func stripPackage(pkg string) string {
	split := strings.Split(pkg, ".")
	return split[len(split)-1]
}

// mustBeValue takes the string representation of a go symbol and strips the pointer
// character (*) if it's there
func mustBeValue(name string) string {
	return strings.Replace(name, "*", "", -1)
}

// mustBePtr takes the string representation of a go symbol and adds the pointer
// character (*) if it's not there
func mustBePtr(name string) string {
	return "*" + mustBeValue(name)
}

// typeInfoSorter allows us to sort type converter functions by the name of the
// input type, then the name of the output type
type typeInfoSorter []typeInfo

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
