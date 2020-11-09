package core

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
	in           types.Type
	nameOverride string
	outIsPtr     bool
	outType      string
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
	typeName := opts.nameOverride
	switch in := opts.in.(type) {
	case *types.Basic:
		info.InType = opts.in.String()
		return simplePtrs(info, generatedConversions)
	case *types.Pointer:
		value, isPrimitive := in.Elem().(*types.Basic)
		if isPrimitive {
			info.InType = value.String()
			info.InIsPtr = true
			return simplePtrs(info, generatedConversions)
		}
		underlyingStruct, isStruct := in.Elem().(*types.Named)
		if isStruct {
			if typeName == "" {
				typeName = underlyingStruct.String()
			}
			return objectTypeFnName(typeName, opts.outIsPtr)
		}
		return convertFuncs{}, errors.New("complex pointers not implemented yet")
	case *types.Named:
		switch in.String() {
		case "time.Time":
			return simplePtrs(info, generatedConversions)
		default:
			if underlying := in.Underlying(); underlying != nil {
				opts.in = underlying
				opts.nameOverride = in.String()
				return conversionFn(opts, generatedConversions)
			}
			if typeName == "" {
				typeName = in.String()
			}
			return objectTypeFnName(typeName, opts.outIsPtr)
		}
	case *types.Map:
		switch in.String() {
		case "map[string]interface{}":
			return simplePtrs(info, generatedConversions)
		default:
			return convertFuncs{}, errors.New("complex maps not implemented yet")
		}
	case *types.Interface:
		switch in.String() {
		case "interface{}":
			return simplePtrs(info, generatedConversions)
		default:
			return convertFuncs{}, errors.New("non-empty interfaces not implemented yet")
		}
	case *types.Slice:
		// no conversion code is generated here for arrays because it's done in generateArrayConversions
		inName := cleanName(shortenPackage(in.String()), containsPtr(in.String()))
		outName := cleanName(shortenPackage(opts.outType), containsPtr(opts.outType))
		return convertFuncs{
			converter: fmt.Sprintf("%s%s", inName, outName),
			inverter:  fmt.Sprintf("%s%s", outName, inName),
		}, nil
	case *types.Array:
		inName := cleanName(shortenPackage(in.String()), containsPtr(in.String()))
		outName := cleanName(shortenPackage(opts.outType), containsPtr(opts.outType))
		return convertFuncs{
			converter: fmt.Sprintf("%s%s", inName, outName),
			inverter:  fmt.Sprintf("%s%s", outName, inName),
		}, nil
	case *types.Struct:
		if typeName == "" {
			typeName = in.String()
		}
		return objectTypeFnName(typeName, opts.outIsPtr)
	default:
		return convertFuncs{}, errors.Errorf("converting type %s is not supported", opts.in.String())
	}
}

// generateArrayConversions does a similar task to generateServiceConversions, but specifically
// generates code which will convert arrays whose elements are arbitrary types. The approach
// here has logical similarities to generating conversion code for both a struct and a scalar
func generateArrayConversions(modelName string, modelType types.Type, restName, restType string, outIsPtr bool, generatedConversions map[typeInfo]string) (string, error) {
	arrayElem := findArrayElem(modelType)
	if arrayElem == nil {
		return "", errors.Errorf("unexpected error: '%s' is not an array", modelName)
	}
	conversionInfo := typeInfo{
		InType: modelType.String(),
	}
	if generatedConversions[conversionInfo] != "" {
		return "", nil
	}

	arrayTemplate, err := getTemplate(arrayMethodsTemplatePath)
	if err != nil {
		return "", errors.Wrap(err, "error getting service methods template")
	}
	lineTemplate, err := getTemplate(funcTemplatePath)
	if err != nil {
		return "", errors.Wrap(err, "error getting line template")
	}
	opts := convertFnOpts{
		in:       modelType,
		outIsPtr: outIsPtr,
		outType:  restType,
	}
	funcs, err := arrayConversionFn(opts, generatedConversions)
	if err != nil {
		return "", err
	}
	bfsData := conversionLine{
		TypeConversionFunc: funcs.converter,
	}
	bfsCode, err := output(lineTemplate, bfsData)
	if err != nil {
		return "", errors.Wrap(err, "error generating line code")
	}
	modelTypeStr := fmt.Sprintf("[]%s", (*arrayElem).String())
	serviceData := arrayConversionInfo{
		FromType:       shortenPackage(modelTypeStr),
		FromHasPtr:     containsPtr(modelTypeStr),
		ToType:         shortenPackage(restType),
		ToHasPtr:       containsPtr(restType),
		ConversionCode: bfsCode,
	}
	code, err := output(arrayTemplate, serviceData)
	if err != nil {
		return "", err
	}
	if modelTypeStr != restType {
		tsData := conversionLine{
			TypeConversionFunc: funcs.inverter,
		}
		tsCode, err := output(lineTemplate, tsData)
		if err != nil {
			return "", errors.Wrap(err, "error generating line code")
		}
		serviceData = arrayConversionInfo{
			FromType:       shortenPackage(restType),
			FromHasPtr:     containsPtr(restType),
			ToType:         shortenPackage(modelTypeStr),
			ToHasPtr:       containsPtr(modelTypeStr),
			ConversionCode: tsCode,
		}
		reverseCode, err := output(arrayTemplate, serviceData)
		if err != nil {
			return "", err
		}
		code += "\n"
		code += reverseCode
	}
	generatedConversions[conversionInfo] = code

	return code, nil
}

// arrayConversionFn is similar to conversionFn above, but specifically takes an array
// type and returns the correct function to convert each element of the array, using the
// naming conventions defined in this file
func arrayConversionFn(opts convertFnOpts, generatedConversions map[typeInfo]string) (convertFuncs, error) {
	switch in := opts.in.(type) {
	case *types.Slice:
		newOpts := opts
		newOpts.in = in.Elem()
		_, newOpts.outType = stripArray(opts.outType)
		return conversionFn(newOpts, generatedConversions)
	case *types.Array:
		newOpts := opts
		newOpts.in = in.Elem()
		_, newOpts.outType = stripArray(opts.outType)
		return conversionFn(newOpts, generatedConversions)
	default:
		return convertFuncs{}, errors.New("programmer error: attempted to convert a non-array function")
	}
}

// objectTypeFnName takes a data type and returns the names of the functions
// used to convert to/from that type in a way that is callable directly in code,
// using the very specific naming conventions defined in this file
func objectTypeFnName(in string, outIsPtr bool) (convertFuncs, error) {
	funcs := convertFuncs{}
	fnName := ""
	if !outIsPtr {
		fnName = "*"
	}
	fnName += "API"
	fnName += stripPackage(in)
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
	re := regexp.MustCompile("^([^A-Za-z]*)([A-Za-z]+.*)$")
	prefix := ""
	suffix := ""
	matches := re.FindStringSubmatch(pkg)
	if len(matches) < 2 {
		return ""
	} else if len(matches) == 2 {
		suffix = matches[1]
	} else {
		prefix = matches[1]
		suffix = matches[2]
	}
	split := strings.Split(suffix, "/")
	return prefix + split[len(split)-1]
}

// shortenPackage takes a package+struct name and strips the package
// for example: myPackage.myStruct becomes myStruct
func stripPackage(pkg string) string {
	split := strings.Split(pkg, ".")
	return split[len(split)-1]
}

// stripArray takes a type name and strips the array symbol, returning
// whether the type was an array as well as the array element's type.
// for example: []myPackage.myStruct returns true, myPackage.myStruct
func stripArray(typeName string) (bool, string) {
	isArray := strings.HasPrefix(typeName, "[]")
	elementName := strings.Replace(typeName, "[]", "", 1)
	return isArray, elementName
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

// containsPtr takes the string representation of a go symbol and returns if it has
// the pointer character (*) in it
func containsPtr(name string) bool {
	return strings.Contains(name, "*")
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
