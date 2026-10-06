package openapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// successAnnotation matches the response type in a swaggo success annotation,
// e.g. "@Success 200 {array} model.APITask".
var successAnnotation = regexp.MustCompile(`@Success\s+\d+\s+\{(?:object|array)\}\s+(\S+)`)

// TestResponseFieldsDeclareNullability requires every REST v2 response field
// that Go can serialize as null to say whether the spec should allow null.
// Without the x-nullable extension, the spec claims these fields are never
// null, which breaks clients generated from it.
func TestResponseFieldsDeclareNullability(t *testing.T) {
	loader, err := newSourceLoader(t)
	require.NoError(t, err)

	roots, err := loader.responseTypes(t, "rest/route")
	require.NoError(t, err)
	require.NotEmpty(t, roots, "should find response types in the route annotations")

	var missing []string
	for _, root := range roots {
		missing = append(missing, loader.undeclaredNullableFields(root)...)
	}
	sort.Strings(missing)
	missing = slices.Compact(missing)

	assert.Empty(t, missing, "These response fields can serialize as null. Add `extensions:\"x-nullable\"` to the struct tag if the field can be nil when returned, or `extensions:\"!x-nullable\"` if it's always set:\n%s", strings.Join(missing, "\n"))
}

// sourceLoader parses Evergreen packages as plain ASTs, resolving types across
// packages by import path. It deliberately avoids type checking (e.g.
// go/packages) because the linter applies the main module's Go language
// version to dependency sources, and current golang.org/x/tools requires a
// newer one.
type sourceLoader struct {
	root  string // repository root directory
	fset  *token.FileSet
	pkgs  map[string]*ast.Package             // by import path, e.g. "rest/model"
	types map[string]map[string]*ast.TypeSpec // by import path, then type name
}

func newSourceLoader(t *testing.T) (*sourceLoader, error) {
	// This test lives in rest/openapi, two levels below the repository root.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return &sourceLoader{root: root, fset: token.NewFileSet(), pkgs: map[string]*ast.Package{}, types: map[string]map[string]*ast.TypeSpec{}}, nil
}

// pkg returns the AST package for an import path relative to the module root,
// e.g. "rest/model", parsing it on first use.
func (l *sourceLoader) pkg(importPath string) (*ast.Package, error) {
	if pkg, ok := l.pkgs[importPath]; ok {
		return pkg, nil
	}
	pkg, err := l.parse(importPath)
	if err != nil {
		return nil, err
	}
	l.pkgs[importPath] = pkg
	return pkg, nil
}

func (l *sourceLoader) parse(importPath string) (*ast.Package, error) {
	pkgs, err := parser.ParseDir(l.fset, filepath.Join(l.root, importPath), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for name, pkg := range pkgs {
		if !strings.HasPrefix(name, "_") {
			return pkg, nil
		}
	}
	return nil, nil
}

// typeRef identifies a struct type by its import path (relative to the module
// root) and name.
type typeRef struct {
	importPath string
	name       string
}

// responseTypes returns the types named in the route package's success
// annotations.
func (l *sourceLoader) responseTypes(t *testing.T, importPath string) ([]typeRef, error) {
	pkg, err := l.pkg(importPath)
	if err != nil {
		return nil, err
	}

	var roots []typeRef
	seen := map[typeRef]bool{}
	for _, file := range pkg.Files {
		imports := l.fileImports(file)
		for _, group := range file.Comments {
			for _, match := range successAnnotation.FindAllStringSubmatch(group.Text(), -1) {
				typeName := strings.TrimPrefix(match[1], "[]")
				if typeName == "string" || typeName == "object" {
					continue
				}
				// Unqualified names refer to types in the route package itself.
				ref, ok := l.resolve(typeName, imports)
				if !ok && !strings.Contains(typeName, ".") {
					ref, ok = typeRef{importPath: importPath, name: typeName}, true
				}
				if !ok {
					// swaggo also resolves packages the file doesn't import;
					// fall back to searching the repo by package name.
					ref, ok = l.resolveLoose(typeName)
				}
				require.True(t, ok, "could not resolve response type '%s'", typeName)
				if !seen[ref] {
					seen[ref] = true
					roots = append(roots, ref)
				}
			}
		}
	}
	return roots, nil
}

// fileImports maps the import qualifiers usable in swaggo annotations to
// import paths. swaggo accepts either the file's import alias or the
// package's own name.
func (l *sourceLoader) fileImports(file *ast.File) map[string]string {
	imports := map[string]string{}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		imports[path] = path
		if imp.Name != nil {
			imports[imp.Name.Name] = path
		} else {
			// Without an alias, swaggo qualifies the import by its package
			// name, which matches the last path element for Evergreen packages.
			imports[filepath.Base(path)] = path
		}
	}
	return imports
}

// resolve resolves a type name as swaggo writes it: qualified by an import
// alias or package name.
func (l *sourceLoader) resolve(typeName string, imports map[string]string) (typeRef, bool) {
	qualifier, name, qualified := strings.Cut(typeName, ".")
	if !qualified {
		return typeRef{}, false
	}
	path, ok := imports[qualifier]
	if !ok {
		return typeRef{}, false
	}
	return typeRef{importPath: toRelative(path), name: name}, true
}

// toRelative converts an import path to a directory relative to the module
// root, which is how this loader addresses packages. Non-module imports are
// left as full paths, which the walking code treats as external.
func toRelative(importPath string) string {
	return strings.TrimPrefix(importPath, "github.com/evergreen-ci/evergreen/")
}

// resolveLoose resolves a qualified type name without a matching import by
// searching the repo directories named after the qualifier, mirroring swaggo's
// dependency-based resolution.
func (l *sourceLoader) resolveLoose(typeName string) (typeRef, bool) {
	qualifier, name, qualified := strings.Cut(typeName, ".")
	if !qualified {
		return typeRef{}, false
	}
	for _, candidate := range []string{qualifier, "rest/" + qualifier, "model/" + qualifier, "pkg/" + qualifier, "service/" + qualifier, "apimodels", "graphql/" + qualifier, "units/" + qualifier, "cloud/" + qualifier, "thirdparty/" + qualifier, "operations/" + qualifier} {
		spec, _, err := l.typeSpec(typeRef{importPath: candidate, name: name})
		if err == nil && spec != nil {
			return typeRef{importPath: candidate, name: name}, true
		}
	}
	return typeRef{}, false
}

func (l *sourceLoader) typeSpec(ref typeRef) (*ast.TypeSpec, *ast.Package, error) {
	if _, ok := l.types[ref.importPath]; !ok {
		pkg, err := l.pkg(ref.importPath)
		if err != nil {
			return nil, nil, err
		}
		specs := map[string]*ast.TypeSpec{}
		if pkg != nil {
			for _, file := range pkg.Files {
				for _, decl := range file.Decls {
					genDecl, ok := decl.(*ast.GenDecl)
					if !ok || genDecl.Tok != token.TYPE {
						continue
					}
					for _, spec := range genDecl.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							specs[ts.Name.Name] = ts
						}
					}
				}
			}
		}
		l.types[ref.importPath] = specs
	}
	spec, ok := l.types[ref.importPath][ref.name]
	if !ok {
		return nil, nil, nil
	}
	return spec, l.pkgs[ref.importPath], nil
}

// undeclaredNullableFields walks a response type and the Evergreen types it
// contains, returning the fields that can be null but don't declare whether
// they're nullable.
func (l *sourceLoader) undeclaredNullableFields(root typeRef) []string {
	var missing []string
	seen := map[typeRef]bool{}
	var visit func(ref typeRef)
	visit = func(ref typeRef) {
		// Relative paths are module-internal; anything else is an external
		// dependency we don't walk.
		if seen[ref] || strings.HasPrefix(ref.importPath, "github.com/") {
			return
		}
		seen[ref] = true
		spec, pkg, err := l.typeSpec(ref)
		if err != nil || spec == nil || pkg == nil {
			return
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			return
		}
		// Types with custom JSON marshalling don't serialize their fields directly.
		if hasMarshalJSON(spec.Name.Name, pkg) {
			return
		}
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
			jsonName, jsonOpts, _ := strings.Cut(tag.Get("json"), ",")
			if jsonName == "-" || tag.Get("swaggerignore") == "true" {
				continue
			}
			if len(field.Names) == 0 && jsonName == "" {
				// Untagged embedded struct fields are flattened into the parent.
				if nested, ok := l.refOf(field.Type, ref.importPath); ok {
					visit(nested)
				}
				continue
			}
			if !isExportedField(field) {
				continue
			}
			if nested, ok := l.refOf(field.Type, ref.importPath); ok {
				visit(nested)
			}

			if !l.canBeNull(field.Type, ref.importPath) || slices.Contains(strings.Split(jsonOpts, ","), "omitempty") {
				continue
			}
			if !declaresNullability(tag) {
				if jsonName == "" {
					jsonName = field.Names[0].Name
				}
				missing = append(missing, ref.importPath+"."+ref.name+"."+field.Names[0].Name+" (json: "+jsonName+")")
			}
		}
	}
	visit(root)
	return missing
}

// refOf resolves an AST type expression to a named Evergreen struct type, if
// it is (or points into) one.
func (l *sourceLoader) refOf(expr ast.Expr, currentPath string) (typeRef, bool) {
	switch typ := expr.(type) {
	case *ast.StarExpr:
		return l.refOf(typ.X, currentPath)
	case *ast.ArrayType:
		return l.refOf(typ.Elt, currentPath)
	case *ast.MapType:
		return l.refOf(typ.Value, currentPath)
	case *ast.Ident:
		if typ.Obj != nil && typ.Obj.Decl != nil {
			if ts, ok := typ.Obj.Decl.(*ast.TypeSpec); ok {
				return typeRef{importPath: currentPath, name: ts.Name.Name}, true
			}
		}
		return typeRef{importPath: currentPath, name: typ.Name}, true
	case *ast.SelectorExpr:
		if ident, ok := typ.X.(*ast.Ident); ok {
			pkg, err := l.pkg(currentPath)
			if err != nil || pkg == nil {
				return typeRef{}, false
			}
			for _, file := range pkg.Files {
				if ref, ok := l.resolve(ident.Name+"."+typ.Sel.Name, l.fileImports(file)); ok {
					return ref, true
				}
			}
		}
	}
	return typeRef{}, false
}

// isExportedField reports whether the field is exported. Embedded fields have
// no name and are always exported.
func isExportedField(field *ast.Field) bool {
	if len(field.Names) == 0 {
		return true
	}
	return field.Names[0].IsExported()
}

// canBeNull returns whether encoding/json can serialize a value of this type
// as null.
func (l *sourceLoader) canBeNull(expr ast.Expr, currentPath string) bool {
	switch expr.(type) {
	case *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.InterfaceType:
		return true
	case *ast.SelectorExpr:
		// json marshals named types by their underlying kind.
		if ref, ok := l.refOf(expr, currentPath); ok {
			if spec, _, err := l.typeSpec(ref); err == nil && spec != nil {
				switch spec.Type.(type) {
				case *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.InterfaceType:
					return true
				}
			}
		}
		return false
	}
	return false
}

func declaresNullability(tag reflect.StructTag) bool {
	for ext := range strings.SplitSeq(tag.Get("extensions"), ",") {
		if ext == "x-nullable" || ext == "!x-nullable" {
			return true
		}
	}
	return false
}

// hasMarshalJSON reports whether the package defines a MarshalJSON method for
// the named type.
func hasMarshalJSON(typeName string, pkg *ast.Package) bool {
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "MarshalJSON" || len(fn.Recv.List) != 1 {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if ident, ok := recv.(*ast.Ident); ok && ident.Name == typeName {
				return true
			}
		}
	}
	return false
}
