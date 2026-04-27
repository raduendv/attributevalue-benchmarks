package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
)

var (
	typeNames  = flag.String("type", "", "comma-separated list of type names; must be set")
	output     = flag.String("output", "", "output file name; default srcdir/<type>_string.go")
	filesuffix = flag.String("filesuffix", "_generated", "suffix for generated files name, will be inserted before extension (before .go)")
	buildTags  = flag.String("tags", "", "comma-separated list of build tags to apply")
	appendMode = flag.Bool("append", false, "append generated methods to the destination file instead of overwriting")
)

/**
GOPATH=/Users/radugribincea/.gvm/pkgsets/go1.26.0/global
GOROOT=/Users/radugribincea/.gvm/gos/go1.26.0
GOARCH=arm64
GOOS=darwin
GOFILE=structs_test.go
GOLINE=12
GOPACKAGE=main
*/

func main() {
	flag.Usage = Usage
	flag.Parse()

	x := os.Environ()
	for _, q := range x {
		if q[0:2] == "GO" {
			println(q)
		}
	}

	if len(*typeNames) == 0 {
		l, _ := strconv.ParseInt(os.Getenv("GOLINE"), 10, 64)
		*typeNames = findType(
			os.Getenv("GOFILE"),
			l,
		)
		if len(*typeNames) == 0 {
			flag.Usage()
			os.Exit(2)
		}
	}

	types := strings.Split(*typeNames, ",")
	if err := processTypes(types, os.Getenv("GOFILE"), *filesuffix, *appendMode); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err.Error())
	}
}

type generatedType struct {
	structName string
	fields     []resolvedField
}

type importRequirement struct {
	Path  string
	Alias string
}

type packageContext struct {
	typeSpecs          map[string]*ast.TypeSpec
	importsByQualifier map[string]importRequirement
}

type packageLoader struct {
	cache map[string]*packageContext
}

func newPackageLoader() *packageLoader {
	return &packageLoader{cache: map[string]*packageContext{}}
}

func (l *packageLoader) load(importPath string) (*packageContext, error) {
	if ctx, ok := l.cache[importPath]; ok {
		return ctx, nil
	}

	dirOut, err := exec.Command("go", "list", "-f", "{{.Dir}}", importPath).Output()
	if err != nil {
		return nil, fmt.Errorf("resolve import %q: %w", importPath, err)
	}

	dir := strings.TrimSpace(string(dirOut))
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi os.FileInfo) bool {
		name := fi.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		return nil, fmt.Errorf("parse package %q: %w", importPath, err)
	}

	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
		break
	}
	if pkg == nil {
		return nil, fmt.Errorf("no package files found for import %q", importPath)
	}

	typeSpecs := map[string]*ast.TypeSpec{}
	importsByQualifier := map[string]importRequirement{}
	for _, f := range pkg.Files {
		for _, decl := range f.Decls {
			d, ok := decl.(*ast.GenDecl)
			if !ok || d.Tok != token.TYPE {
				continue
			}

			for _, spec := range d.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name == nil {
					continue
				}

				typeSpecs[typeSpec.Name.Name] = typeSpec
			}
		}

		for q, req := range mapImportsByQualifier(f) {
			if _, exists := importsByQualifier[q]; !exists {
				importsByQualifier[q] = req
			}
		}
	}

	ctx := &packageContext{typeSpecs: typeSpecs, importsByQualifier: importsByQualifier}
	l.cache[importPath] = ctx
	return ctx, nil
}

func processTypes(typeNames []string, p, s string, appendMode bool) error {
	parts := strings.Split(p, ".")
	fileName := parts[len(parts)-2]
	if strings.HasSuffix(fileName, "_test") {
		parts[len(parts)-2] = fmt.Sprintf("%s%s_test", parts[len(parts)-2][0:len(fileName)-5], s)
	} else {
		parts[len(parts)-2] = fmt.Sprintf("%s%s", parts[len(parts)-2], s)
	}

	dst := os.Getenv("PWD") + string(os.PathSeparator) + strings.Join(parts, ".")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, p, nil, 0)
	if err != nil {
		return err
	}

	typeSpecs := make(map[string]*ast.TypeSpec)
	importsByQualifier := mapImportsByQualifier(file)
	for _, decl := range file.Decls {
		d, ok := decl.(*ast.GenDecl)
		if !ok || d.Tok != token.TYPE {
			continue
		}

		for _, spec := range d.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil {
				continue
			}

			typeSpecs[typeSpec.Name.Name] = typeSpec
		}
	}

	generated := []generatedType{}
	needsStrconv := false
	needsAttributeValue := false
	needsUtil := false
	extraImports := map[string]importRequirement{}
	loader := newPackageLoader()
	for _, t := range typeNames {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}

		selectedTypeSpec := (*ast.TypeSpec)(nil)
		for _, decl := range file.Decls {
			d, ok := decl.(*ast.GenDecl)
			if !ok || d.Tok != token.TYPE {
				continue
			}

			for _, spec := range d.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name == nil || fmt.Sprintf("%s.%s", file.Name.Name, typeSpec.Name.Name) != t {
					continue
				}

				selectedTypeSpec = typeSpec
				break
			}

			if selectedTypeSpec != nil {
				break
			}
		}

		if selectedTypeSpec == nil {
			return fmt.Errorf("type %q not found in %s", t, p)
		}

		resolvedFields, err := collectAllFields(selectedTypeSpec.Type, "", typeSpecs, importsByQualifier, loader, map[string]ast.Expr{}, map[string]bool{})
		if err != nil {
			return err
		}

		generated = append(generated, generatedType{
			structName: selectedTypeSpec.Name.Name,
			fields:     resolvedFields,
		})

		for _, field := range resolvedFields {
			if field.PointerDepth > 0 {
				needsUtil = true
			}

			for _, req := range field.Imports {
				extraImports[importRequirementKey(req)] = req
			}

			if isListOrMapType(field.BaseType) {
				spec, direct := collectionSpecForType(field.BaseType)
				if !direct {
					needsAttributeValue = true
				} else {
					if suffix, ok := attributeValueMemberFor(spec.ElemType); ok && suffix == "N" {
						needsStrconv = true
					}
				}
				continue
			}

			memberSuffix, ok := attributeValueMemberFor(field.BaseType)
			if ok && memberSuffix == "N" {
				needsStrconv = true
				continue
			}

			if !ok {
				needsAttributeValue = true
			}
		}

		if needsStrconv {
			continue
		}
	}

	existingSize := int64(0)
	if info, statErr := os.Stat(dst); statErr == nil {
		existingSize = info.Size()
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	writeHeader := !appendMode || existingSize == 0

	openFlags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		openFlags |= os.O_APPEND
	} else {
		openFlags |= os.O_TRUNC
	}

	f, err := os.OpenFile(dst, openFlags, 0o644)
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	if err != nil {
		return err
	}

	if appendMode && !writeHeader {
		requiredImports := []importRequirement{{Path: "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"}}
		if needsStrconv {
			requiredImports = append(requiredImports, importRequirement{Path: "strconv"})
		}
		if needsUtil {
			requiredImports = append(requiredImports, importRequirement{Path: "pkg/util"})
		}
		if needsAttributeValue {
			requiredImports = append(requiredImports, importRequirement{Path: "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"})
		}
		for _, req := range extraImports {
			requiredImports = append(requiredImports, req)
		}

		if err := ensureImports(dst, requiredImports); err != nil {
			return err
		}

		f.WriteString("\n")
	}

	if writeHeader {
		written := map[string]bool{}
		f.WriteString(fmt.Sprintf("package %s\n\n", file.Name.Name))
		f.WriteString("import \"github.com/aws/aws-sdk-go-v2/service/dynamodb/types\"\n")
		written[importRequirementKey(importRequirement{Path: "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"})] = true
		if needsStrconv {
			f.WriteString("import \"strconv\"\n")
			written[importRequirementKey(importRequirement{Path: "strconv"})] = true
		}
		if needsUtil {
			f.WriteString("import \"pkg/util\"\n")
			written[importRequirementKey(importRequirement{Path: "pkg/util"})] = true
		}
		if needsAttributeValue {
			f.WriteString("import \"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue\"\n")
			written[importRequirementKey(importRequirement{Path: "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"})] = true
		}
		for _, req := range extraImports {
			if written[importRequirementKey(req)] || written[req.Path] {
				continue
			}
			if req.Alias != "" {
				f.WriteString(req.Alias + " \"" + req.Path + "\"\n")
			} else {
				f.WriteString("import \"" + req.Path + "\"\n")
			}
			written[importRequirementKey(req)] = true
			written[req.Path] = true
		}
		f.WriteString("\n")
	}

	for i, g := range generated {
		f.WriteString(fmt.Sprintf(
			"func (ss *%s) UnmarshalDynamoDBAttributeValue(in types.AttributeValue) error {\n"+
				"\tm := in.(*types.AttributeValueMemberM).Value\n\n",
			g.structName,
		))

		for _, field := range g.fields {
			if isListOrMapType(field.BaseType) {
				if spec, ok := collectionSpecForType(field.BaseType); ok {
					valueExpr := decodeCollectionValueExpr(spec, "raw")
					valueExpr = wrapCall("util.Pointer", valueExpr, field.PointerDepth)
					f.WriteString(fmt.Sprintf(
						"\tif raw, ok := m[%q]; ok && raw != nil {\n"+
							"\t\tss.%s = %s\n"+
							"\t}\n",
						field.DynamoName,
						field.Selector,
						valueExpr,
					))
					continue
				}

				valueExpr := fmt.Sprintf("func() %s { var out %s; _ = attributevalue.Unmarshal(raw, &out); return out }()", field.BaseType, field.BaseType)
				valueExpr = wrapCall("util.Pointer", valueExpr, field.PointerDepth)
				f.WriteString(fmt.Sprintf(
					"\tif raw, ok := m[%q]; ok && raw != nil {\n"+
						"\t\tss.%s = %s\n"+
						"\t}\n",
					field.DynamoName,
					field.Selector,
					valueExpr,
				))
				continue
			}

			memberSuffix, ok := attributeValueMemberFor(field.BaseType)
			if !ok {
				valueExpr := fmt.Sprintf("func() %s { var out %s; _ = attributevalue.Unmarshal(raw, &out); return out }()", field.BaseType, field.BaseType)
				valueExpr = wrapCall("util.Pointer", valueExpr, field.PointerDepth)
				f.WriteString(fmt.Sprintf(
					"\tif raw, ok := m[%q]; ok && raw != nil {\n"+
						"\t\tss.%s = %s\n"+
						"\t}\n",
					field.DynamoName,
					field.Selector,
					valueExpr,
				))
				continue
			}

			valueExpr := "mv.Value"
			valueExpr = decodeMemberValue(field.BaseType, valueExpr)
			valueExpr = wrapCall("util.Pointer", valueExpr, field.PointerDepth)

			f.WriteString(fmt.Sprintf(
				"\tif raw, ok := m[%q]; ok && raw != nil {\n"+
					"\t\tif mv, ok := raw.(*types.AttributeValueMember%s); ok {\n"+
					"\t\t\tss.%s = %s\n"+
					"\t\t}\n"+
					"\t}\n",
				field.DynamoName,
				memberSuffix,
				field.Selector,
				valueExpr,
			))
		}

		f.WriteString("\n")
		f.WriteString("\treturn nil\n")
		f.WriteString("}\n")
		f.WriteString("\n")

		f.WriteString(fmt.Sprintf(
			"func (ss *%s) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {\n"+
				"\tout := make(map[string]types.AttributeValue, %d)\n\n",
			g.structName,
			len(g.fields),
		))

		for _, field := range g.fields {
			valueExpr := wrapCall("util.Unwrap", "ss."+field.Selector, field.PointerDepth)
			if isListOrMapType(field.BaseType) {
				if spec, ok := collectionSpecForType(field.BaseType); ok {
					f.WriteString(fmt.Sprintf("\tout[%q] = %s\n", field.DynamoName, encodeCollectionValueExpr(spec, valueExpr)))
					continue
				}

				f.WriteString(fmt.Sprintf(
					"\tif av, err := attributevalue.Marshal(%s); err != nil {\n"+
						"\t\treturn nil, err\n"+
						"\t} else {\n"+
						"\t\tout[%q] = av\n"+
						"\t}\n",
					valueExpr,
					field.DynamoName,
				))
				continue
			}

			memberSuffix, ok := attributeValueMemberFor(field.BaseType)
			if !ok {
				f.WriteString(fmt.Sprintf(
					"\tif av, err := attributevalue.Marshal(%s); err != nil {\n"+
						"\t\treturn nil, err\n"+
						"\t} else {\n"+
						"\t\tout[%q] = av\n"+
						"\t}\n",
					valueExpr,
					field.DynamoName,
				))
				continue
			}

			valueExpr = encodeMemberValue(field.BaseType, valueExpr)
			f.WriteString(fmt.Sprintf(
				"\tout[%q] = &types.AttributeValueMember%s{\n"+
					"\t\tValue: %s,\n"+
					"\t}\n",
				field.DynamoName,
				memberSuffix,
				valueExpr,
			))
		}

		f.WriteString("\n")
		f.WriteString("\treturn &types.AttributeValueMemberM{Value: out}, nil\n")
		f.WriteString("}\n")
		if i < len(generated)-1 {
			f.WriteString("\n")
		}
	}

	return nil
}

func isListOrMapType(baseType string) bool {
	if baseType == "[]byte" {
		return false
	}

	return strings.HasPrefix(baseType, "[]") || strings.HasPrefix(baseType, "map[")
}

type collectionSpec struct {
	Kind     string
	ElemType string
}

func collectionSpecForType(baseType string) (collectionSpec, bool) {
	if strings.HasPrefix(baseType, "[]") {
		elem := strings.TrimSpace(strings.TrimPrefix(baseType, "[]"))
		if _, ok := attributeValueMemberFor(elem); ok {
			return collectionSpec{Kind: "list", ElemType: elem}, true
		}

		return collectionSpec{}, false
	}

	if !strings.HasPrefix(baseType, "map[") {
		return collectionSpec{}, false
	}

	end := strings.Index(baseType, "]")
	if end < 0 {
		return collectionSpec{}, false
	}

	keyType := strings.TrimSpace(baseType[len("map["):end])
	if keyType != "string" {
		return collectionSpec{}, false
	}

	elem := strings.TrimSpace(baseType[end+1:])
	if _, ok := attributeValueMemberFor(elem); ok {
		return collectionSpec{Kind: "map", ElemType: elem}, true
	}

	return collectionSpec{}, false
}

func decodeCollectionValueExpr(spec collectionSpec, avExpr string) string {
	suffix, _ := attributeValueMemberFor(spec.ElemType)

	decodedElem := decodeMemberValue(spec.ElemType, "mv.Value")
	if spec.Kind == "list" {
		return fmt.Sprintf(
			"func() []%s { av, ok := %s.(*types.AttributeValueMemberL); if !ok { return nil }; out := make([]%s, len(av.Value)); for i, item := range av.Value { if mv, ok := item.(*types.AttributeValueMember%s); ok { out[i] = %s } }; return out }()",
			spec.ElemType,
			avExpr,
			spec.ElemType,
			suffix,
			decodedElem,
		)
	}

	return fmt.Sprintf(
		"func() map[string]%s { av, ok := %s.(*types.AttributeValueMemberM); if !ok { return nil }; out := make(map[string]%s, len(av.Value)); for k, item := range av.Value { if mv, ok := item.(*types.AttributeValueMember%s); ok { out[k] = %s } }; return out }()",
		spec.ElemType,
		avExpr,
		spec.ElemType,
		suffix,
		decodedElem,
	)
}

func encodeCollectionValueExpr(spec collectionSpec, valueExpr string) string {
	suffix, _ := attributeValueMemberFor(spec.ElemType)

	encodedElem := encodeMemberValue(spec.ElemType, "v")
	if spec.Kind == "list" {
		return fmt.Sprintf(
			"&types.AttributeValueMemberL{Value: func() []types.AttributeValue { if %s == nil { return nil }; out := make([]types.AttributeValue, 0, len(%s)); for _, v := range %s { out = append(out, &types.AttributeValueMember%s{Value: %s}) }; return out }()}",
			valueExpr,
			valueExpr,
			valueExpr,
			suffix,
			encodedElem,
		)
	}

	return fmt.Sprintf(
		"&types.AttributeValueMemberM{Value: func() map[string]types.AttributeValue { if %s == nil { return nil }; out := make(map[string]types.AttributeValue, len(%s)); for k, v := range %s { out[k] = &types.AttributeValueMember%s{Value: %s} }; return out }()}",
		valueExpr,
		valueExpr,
		valueExpr,
		suffix,
		encodedElem,
	)
}

type resolvedField struct {
	Selector     string
	DynamoName   string
	PointerDepth int
	BaseType     string
	Imports      []importRequirement
}

func collectAllFields(expr ast.Expr, prefix string, typeSpecs map[string]*ast.TypeSpec, importsByQualifier map[string]importRequirement, loader *packageLoader, typeArgs map[string]ast.Expr, seen map[string]bool) ([]resolvedField, error) {
	resolvedExpr := substituteTypeArgs(expr, typeArgs)

	switch e := resolvedExpr.(type) {
	case *ast.StructType:
		if e.Fields == nil {
			return nil, nil
		}

		out := []resolvedField{}
		for _, field := range e.Fields.List {
			fieldType := substituteTypeArgs(field.Type, typeArgs)

			if len(field.Names) == 0 {
				embeddedName := embeddedTypeName(fieldType)
				if embeddedName == "" {
					continue
				}

				nestedPrefix := joinSelector(prefix, embeddedName)
				nested, err := collectAllFields(fieldType, nestedPrefix, typeSpecs, importsByQualifier, loader, typeArgs, seen)
				if err != nil {
					return nil, err
				}
				out = append(out, nested...)
				continue
			}

			for _, name := range field.Names {
				if name == nil {
					continue
				}

				dynamoName, skip := dynamoFieldName(field.Tag, name.Name)
				if skip {
					continue
				}

				depth, base := pointerDepthAndBaseType(fieldType)
				out = append(out, resolvedField{
					Selector:     joinSelector(prefix, name.Name),
					DynamoName:   dynamoName,
					PointerDepth: depth,
					BaseType:     base,
					Imports:      importsForExpr(fieldType, importsByQualifier),
				})
			}
		}

		return out, nil

	case *ast.ParenExpr:
		return collectAllFields(e.X, prefix, typeSpecs, importsByQualifier, loader, typeArgs, seen)
	case *ast.StarExpr:
		return collectAllFields(e.X, prefix, typeSpecs, importsByQualifier, loader, typeArgs, seen)
	case *ast.IndexExpr:
		return collectAllFieldsFromTypeRef(e.X, []ast.Expr{e.Index}, prefix, typeSpecs, importsByQualifier, loader, typeArgs, seen)
	case *ast.IndexListExpr:
		return collectAllFieldsFromTypeRef(e.X, e.Indices, prefix, typeSpecs, importsByQualifier, loader, typeArgs, seen)
	case *ast.Ident:
		return collectAllFieldsFromTypeRef(e, nil, prefix, typeSpecs, importsByQualifier, loader, typeArgs, seen)
	case *ast.SelectorExpr:
		return collectAllFieldsFromTypeRef(e, nil, prefix, typeSpecs, importsByQualifier, loader, typeArgs, seen)
	default:
		return nil, nil
	}
}

func collectAllFieldsFromTypeRef(typeRef ast.Expr, args []ast.Expr, prefix string, typeSpecs map[string]*ast.TypeSpec, importsByQualifier map[string]importRequirement, loader *packageLoader, typeArgs map[string]ast.Expr, seen map[string]bool) ([]resolvedField, error) {
	var (
		typeSpec      *ast.TypeSpec
		typeIdentity  string
		nextTypeSpecs map[string]*ast.TypeSpec
		nextImports   map[string]importRequirement
	)

	switch ref := typeRef.(type) {
	case *ast.Ident:
		resolved, ok := typeSpecs[ref.Name]
		if !ok {
			return nil, nil
		}

		typeSpec = resolved
		typeIdentity = ref.Name
		nextTypeSpecs = typeSpecs
		nextImports = importsByQualifier
	case *ast.SelectorExpr:
		pkgIdent, ok := ref.X.(*ast.Ident)
		if !ok {
			return nil, nil
		}

		req, ok := importsByQualifier[pkgIdent.Name]
		if !ok {
			return nil, nil
		}

		pkgCtx, err := loader.load(req.Path)
		if err != nil {
			return nil, err
		}

		resolved, ok := pkgCtx.typeSpecs[ref.Sel.Name]
		if !ok {
			return nil, nil
		}

		typeSpec = resolved
		typeIdentity = req.Path + "." + ref.Sel.Name
		nextTypeSpecs = pkgCtx.typeSpecs
		nextImports = pkgCtx.importsByQualifier
	default:
		return nil, nil
	}

	resolvedArgs := make([]ast.Expr, 0, len(args))
	for _, arg := range args {
		resolvedArgs = append(resolvedArgs, substituteTypeArgs(arg, typeArgs))
	}

	visitKey := typeIdentity + "[" + typeArgsKey(resolvedArgs) + "]"
	if seen[visitKey] {
		return nil, nil
	}

	nextSeen := copySeen(seen)
	nextSeen[visitKey] = true

	nextTypeArgs := copyTypeArgs(typeArgs)
	if typeSpec.TypeParams != nil {
		argIndex := 0
		for _, param := range typeSpec.TypeParams.List {
			for _, name := range param.Names {
				if argIndex >= len(resolvedArgs) {
					break
				}
				nextTypeArgs[name.Name] = resolvedArgs[argIndex]
				argIndex++
			}
		}
	}

	return collectAllFields(typeSpec.Type, prefix, nextTypeSpecs, nextImports, loader, nextTypeArgs, nextSeen)
}

func importsForExpr(expr ast.Expr, importsByQualifier map[string]importRequirement) []importRequirement {
	out := []importRequirement{}
	seen := map[string]bool{}

	for _, q := range extractPackageQualifiersFromExpr(expr) {
		req, ok := importsByQualifier[q]
		if !ok {
			continue
		}

		key := importRequirementKey(req)
		if seen[key] {
			continue
		}

		seen[key] = true
		out = append(out, req)
	}

	return out
}

func embeddedTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return embeddedTypeName(e.X)
	case *ast.ParenExpr:
		return embeddedTypeName(e.X)
	case *ast.IndexExpr:
		return embeddedTypeName(e.X)
	case *ast.IndexListExpr:
		return embeddedTypeName(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

func pointerDepthAndBaseType(expr ast.Expr) (int, string) {
	resolved := expr
	depth := 0
	for {
		star, ok := resolved.(*ast.StarExpr)
		if !ok {
			break
		}

		depth++
		resolved = star.X
	}

	return depth, typeExprName(resolved)
}

func typeExprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok {
			return pkg.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeExprName(e.Elt)
	case *ast.MapType:
		return "map[" + typeExprName(e.Key) + "]" + typeExprName(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.IndexExpr:
		return typeExprName(e.X)
	case *ast.IndexListExpr:
		return typeExprName(e.X)
	case *ast.ParenExpr:
		return typeExprName(e.X)
	case *ast.StarExpr:
		return "*" + typeExprName(e.X)
	default:
		return "unknown"
	}
}

func substituteTypeArgs(expr ast.Expr, typeArgs map[string]ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.Ident:
		mapped, ok := typeArgs[e.Name]
		if !ok {
			return expr
		}

		// Resolve chained substitutions, e.g. T -> U -> string.
		return substituteTypeArgs(mapped, typeArgs)
	case *ast.StarExpr:
		return &ast.StarExpr{Star: e.Star, X: substituteTypeArgs(e.X, typeArgs)}
	case *ast.ArrayType:
		return &ast.ArrayType{Lbrack: e.Lbrack, Len: e.Len, Elt: substituteTypeArgs(e.Elt, typeArgs)}
	case *ast.MapType:
		return &ast.MapType{Map: e.Map, Key: substituteTypeArgs(e.Key, typeArgs), Value: substituteTypeArgs(e.Value, typeArgs)}
	case *ast.ParenExpr:
		return &ast.ParenExpr{Lparen: e.Lparen, X: substituteTypeArgs(e.X, typeArgs), Rparen: e.Rparen}
	case *ast.IndexExpr:
		return &ast.IndexExpr{X: substituteTypeArgs(e.X, typeArgs), Lbrack: e.Lbrack, Index: substituteTypeArgs(e.Index, typeArgs), Rbrack: e.Rbrack}
	case *ast.IndexListExpr:
		indices := make([]ast.Expr, 0, len(e.Indices))
		for _, idx := range e.Indices {
			indices = append(indices, substituteTypeArgs(idx, typeArgs))
		}

		return &ast.IndexListExpr{X: substituteTypeArgs(e.X, typeArgs), Lbrack: e.Lbrack, Indices: indices, Rbrack: e.Rbrack}
	default:
		return expr
	}
}

func copySeen(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range in {
		out[k] = v
	}

	return out
}

func copyTypeArgs(in map[string]ast.Expr) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	for k, v := range in {
		out[k] = v
	}

	return out
}

func typeArgsKey(args []ast.Expr) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, typeExprName(arg))
	}

	return strings.Join(parts, ",")
}

func joinSelector(prefix, name string) string {
	if prefix == "" {
		return name
	}

	return prefix + "." + name
}

func dynamoFieldName(tagLit *ast.BasicLit, fallback string) (string, bool) {
	if tagLit == nil {
		return fallback, false
	}

	unquoted, err := strconv.Unquote(tagLit.Value)
	if err != nil {
		return fallback, false
	}

	v := reflect.StructTag(unquoted).Get("dynamodbav")
	if v == "" {
		return fallback, false
	}

	parts := strings.Split(v, ",")
	if len(parts) == 0 {
		return fallback, false
	}

	if parts[0] == "-" {
		return "", true
	}

	if parts[0] == "" {
		return fallback, false
	}

	return parts[0], false
}

func attributeValueMemberFor(baseType string) (string, bool) {
	switch baseType {
	case "string":
		return "S", true
	case "bool":
		return "BOOL", true
	case "[]byte":
		return "B", true
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "byte", "rune":
		return "N", true
	default:
		return "", false
	}
}

func decodeMemberValue(baseType, valueExpr string) string {
	switch baseType {
	case "string":
		return valueExpr
	case "bool":
		return valueExpr
	case "int":
		return "func() int { v, _ := strconv.ParseInt(" + valueExpr + ", 10, strconv.IntSize); return int(v) }()"
	case "int8":
		return "func() int8 { v, _ := strconv.ParseInt(" + valueExpr + ", 10, 8); return int8(v) }()"
	case "int16":
		return "func() int16 { v, _ := strconv.ParseInt(" + valueExpr + ", 10, 16); return int16(v) }()"
	case "int32", "rune":
		return "func() int32 { v, _ := strconv.ParseInt(" + valueExpr + ", 10, 32); return int32(v) }()"
	case "int64":
		return "func() int64 { v, _ := strconv.ParseInt(" + valueExpr + ", 10, 64); return v }()"
	case "uint":
		return "func() uint { v, _ := strconv.ParseUint(" + valueExpr + ", 10, strconv.IntSize); return uint(v) }()"
	case "uint8", "byte":
		return "func() uint8 { v, _ := strconv.ParseUint(" + valueExpr + ", 10, 8); return uint8(v) }()"
	case "uint16":
		return "func() uint16 { v, _ := strconv.ParseUint(" + valueExpr + ", 10, 16); return uint16(v) }()"
	case "uint32":
		return "func() uint32 { v, _ := strconv.ParseUint(" + valueExpr + ", 10, 32); return uint32(v) }()"
	case "uint64":
		return "func() uint64 { v, _ := strconv.ParseUint(" + valueExpr + ", 10, 64); return v }()"
	case "float32":
		return "func() float32 { v, _ := strconv.ParseFloat(" + valueExpr + ", 32); return float32(v) }()"
	case "float64":
		return "func() float64 { v, _ := strconv.ParseFloat(" + valueExpr + ", 64); return v }()"
	default:
		return valueExpr
	}
}

func encodeMemberValue(baseType, valueExpr string) string {
	switch baseType {
	case "string":
		return valueExpr
	case "bool":
		return valueExpr
	case "int":
		return "strconv.FormatInt(int64(" + valueExpr + "), 10)"
	case "int8":
		return "strconv.FormatInt(int64(" + valueExpr + "), 10)"
	case "int16":
		return "strconv.FormatInt(int64(" + valueExpr + "), 10)"
	case "int32", "rune":
		return "strconv.FormatInt(int64(" + valueExpr + "), 10)"
	case "int64":
		return "strconv.FormatInt(" + valueExpr + ", 10)"
	case "uint":
		return "strconv.FormatUint(uint64(" + valueExpr + "), 10)"
	case "uint8", "byte":
		return "strconv.FormatUint(uint64(" + valueExpr + "), 10)"
	case "uint16":
		return "strconv.FormatUint(uint64(" + valueExpr + "), 10)"
	case "uint32":
		return "strconv.FormatUint(uint64(" + valueExpr + "), 10)"
	case "uint64":
		return "strconv.FormatUint(" + valueExpr + ", 10)"
	case "float32":
		return "strconv.FormatFloat(float64(" + valueExpr + "), 'f', -1, 32)"
	case "float64":
		return "strconv.FormatFloat(" + valueExpr + ", 'f', -1, 64)"
	default:
		return valueExpr
	}
}

func wrapCall(fn, expr string, depth int) string {
	out := expr
	for i := 0; i < depth; i++ {
		out = fn + "(" + out + ")"
	}

	return out
}

func ensureImports(path string, required []importRequirement) error {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(contentBytes)

	parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if parseErr != nil {
		return parseErr
	}

	needed := []importRequirement{}
	for _, req := range required {
		if !hasImportSpec(parsed.Imports, req) {
			needed = append(needed, req)
		}
	}

	if len(needed) == 0 {
		return nil
	}

	if idx := strings.Index(content, "import ("); idx >= 0 {
		closeIdx := strings.Index(content[idx:], "\n)")
		if closeIdx < 0 {
			return fmt.Errorf("failed to locate end of import block in %s", path)
		}

		insertPos := idx + closeIdx + 1
		inserts := ""
		for _, req := range needed {
			if req.Alias != "" {
				inserts += "\t" + req.Alias + " \"" + req.Path + "\"\n"
			} else {
				inserts += "\t\"" + req.Path + "\"\n"
			}
		}

		content = content[:insertPos] + inserts + content[insertPos:]
		return os.WriteFile(path, []byte(content), 0o644)
	}

	lines := strings.Split(content, "\n")
	firstImport := -1
	lastImport := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import \"") {
			if firstImport == -1 {
				firstImport = i
			}
			lastImport = i
			continue
		}

		if firstImport != -1 {
			break
		}
	}

	if firstImport >= 0 {
		imports := []string{}
		for i := firstImport; i <= lastImport; i++ {
			trimmed := strings.TrimSpace(lines[i])
			imports = append(imports, strings.TrimSuffix(strings.TrimPrefix(trimmed, "import \""), "\""))
		}

		for _, req := range needed {
			if req.Alias != "" {
				imports = append(imports, req.Alias+" \""+req.Path+"\"")
			} else {
				imports = append(imports, req.Path)
			}
		}

		replacement := []string{"import ("}
		for _, imp := range imports {
			if strings.Contains(imp, "\"") {
				replacement = append(replacement, "\t"+imp)
			} else {
				replacement = append(replacement, "\t\""+imp+"\"")
			}
		}
		replacement = append(replacement, ")")

		newLines := append([]string{}, lines[:firstImport]...)
		newLines = append(newLines, replacement...)
		newLines = append(newLines, lines[lastImport+1:]...)

		return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o644)
	}

	insertAt := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			insertAt = i + 1
			break
		}
	}
	if insertAt < 0 {
		return fmt.Errorf("failed to locate package declaration in %s", path)
	}

	block := []string{"", "import ("}
	for _, req := range needed {
		if req.Alias != "" {
			block = append(block, "\t"+req.Alias+" \""+req.Path+"\"")
		} else {
			block = append(block, "\t\""+req.Path+"\"")
		}
	}
	block = append(block, ")")

	newLines := append([]string{}, lines[:insertAt]...)
	newLines = append(newLines, block...)
	newLines = append(newLines, lines[insertAt:]...)

	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o644)
}

func hasImportSpec(imports []*ast.ImportSpec, req importRequirement) bool {
	for _, imp := range imports {
		if imp == nil || imp.Path == nil {
			continue
		}

		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != req.Path {
			continue
		}

		if req.Alias == "" {
			return true
		}

		if imp.Name != nil && imp.Name.Name == req.Alias {
			return true
		}
	}

	return false
}

func mapImportsByQualifier(file *ast.File) map[string]importRequirement {
	out := map[string]importRequirement{}

	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}

		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path == "" {
			continue
		}

		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				continue
			}

			out[imp.Name.Name] = importRequirement{Path: path, Alias: imp.Name.Name}
			continue
		}

		parts := strings.Split(path, "/")
		qualifier := parts[len(parts)-1]
		out[qualifier] = importRequirement{Path: path}
	}

	return out
}

func extractPackageQualifiersFromExpr(expr ast.Expr) []string {
	out := []string{}
	seen := map[string]bool{}

	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		switch v := e.(type) {
		case *ast.SelectorExpr:
			if id, ok := v.X.(*ast.Ident); ok {
				if !seen[id.Name] {
					seen[id.Name] = true
					out = append(out, id.Name)
				}
			} else {
				walk(v.X)
			}
		case *ast.StarExpr:
			walk(v.X)
		case *ast.ArrayType:
			walk(v.Elt)
		case *ast.MapType:
			walk(v.Key)
			walk(v.Value)
		case *ast.IndexExpr:
			walk(v.X)
			walk(v.Index)
		case *ast.IndexListExpr:
			walk(v.X)
			for _, idx := range v.Indices {
				walk(idx)
			}
		case *ast.ParenExpr:
			walk(v.X)
		}
	}

	walk(expr)
	return out
}

func extractPackageQualifiers(typeExpr string) []string {
	out := []string{}
	seen := map[string]bool{}
	start := -1

	flush := func(end int, hasDot bool) {
		if start < 0 || !hasDot {
			start = -1
			return
		}

		ident := typeExpr[start:end]
		if ident != "" && !seen[ident] {
			seen[ident] = true
			out = append(out, ident)
		}

		start = -1
	}

	hasDot := false
	for i, r := range typeExpr {
		isIdentChar := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (start >= 0 && r >= '0' && r <= '9')
		if start == -1 {
			if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				start = i
				hasDot = false
			}
			continue
		}

		if isIdentChar {
			continue
		}

		if r == '.' {
			hasDot = true
			flush(i, true)
			continue
		}

		flush(i, hasDot)
	}

	if start != -1 && hasDot {
		flush(len(typeExpr), true)
	}

	return out
}

func importRequirementKey(req importRequirement) string {
	if req.Alias != "" {
		return req.Alias + ":" + req.Path
	}

	return req.Path
}

func findType(p string, l int64) string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, p, nil, 0)
	if err != nil || file == nil || file.Name == nil {
		return ""
	}

	pkgName := file.Name.Name
	target := int(l)

	closestAfterLine := 0
	closestAfterName := ""
	closestBeforeLine := 0
	closestBeforeName := ""

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}

		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil {
				continue
			}

			line := fset.Position(typeSpec.Pos()).Line
			if line >= target {
				if closestAfterLine == 0 || line < closestAfterLine {
					closestAfterLine = line
					closestAfterName = typeSpec.Name.Name
				}
				continue
			}

			if line > closestBeforeLine {
				closestBeforeLine = line
				closestBeforeName = typeSpec.Name.Name
			}
		}
	}

	if closestAfterName != "" {
		return pkgName + "." + closestAfterName
	}

	if closestBeforeName != "" {
		return pkgName + "." + closestBeforeName
	}

	return ""
}

func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of ravgen:\n")
	fmt.Fprintf(os.Stderr, "\t@TODO")
	flag.PrintDefaults()
}

type File struct {
	Path string
	Node ast.Node
}
