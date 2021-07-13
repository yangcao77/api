package overrides

import (
	"bytes"
	"fmt"
	"go/ast"
	"os"

	"go/printer"
	"go/token"
	"io"
	"regexp"
	"strings"

	"golang.org/x/tools/go/ast/astutil"

	"github.com/devfile/api/generator/genutils"
	"github.com/go-toolsmith/astcopy"
	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"

	"github.com/elliotchance/orderedmap"
)

//go:generate go run sigs.k8s.io/controller-tools/cmd/helpgen generate:headerFile=../header.go.txt,year=2020 paths=.

// +controllertools:marker:generateHelp:category=Overrides

// FieldOverridesInclude drives whether a field should be overriden in devfile parent or plugins
type FieldOverridesInclude struct {
	// Omit indicates that this field cannot be overridden at all.
	Omit bool `marker:",optional"`
	// OmmitInPlugin indicates that this field cannot be overridden in a devfile plugin.
	OmitInPlugin bool `marker:",optional"`
	// Description indicates the description that should be added as Go documentation on the generated structs.
	Description string `marker:",optional"`
}

var (
	overridesFieldMarker = markers.Must(markers.MakeDefinition("devfile:overrides:include", markers.DescribesField, FieldOverridesInclude{}))
	overridesTypeMarker  = markers.Must(markers.MakeDefinition("devfile:overrides:generate", markers.DescribesType, struct{}{}))
)

// +controllertools:marker:generateHelp

// Generator generates additional GO code for the overriding of elements in devfile parent or plugins.
type Generator struct {

	// IsForPluginOverrides indicates that the generated code should be done for plugin overrides.
	// When false, the parent overrides are generated
	IsForPluginOverrides bool `marker:"isForPluginOverrides,optional"`

	suffix            string
	rootTypeToProcess typeToProcess
}

// RegisterMarkers registers the markers of the Generator
func (Generator) RegisterMarkers(into *markers.Registry) error {
	if err := markers.RegisterAll(into, overridesFieldMarker, overridesTypeMarker); err != nil {
		return err
	}
	into.AddHelp(overridesFieldMarker, FieldOverridesInclude{}.Help())
	into.AddHelp(overridesTypeMarker, markers.SimpleHelp("Overrides", "indicates that a type should be selected to create Overrides for it"))
	return genutils.RegisterUnionMarkers(into)
}

// ValueCallback is a callback called for each raw AST (gendecl, typespec) combo.
type ValueCallback func(file *ast.File, decl *ast.GenDecl, spec *ast.ValueSpec)

// EachType calls the given callback for each (gendecl, typespec) combo in the
// given package.  Generally, using markers.EachType is better when working
// with marker data, and has a more convinient representation.
func eachValue(pkg *loader.Package, cb ValueCallback) {
	visitor := &valueVisitor{
		callback: cb,
	}
	pkg.NeedSyntax()
	for _, file := range pkg.Syntax {
		visitor.file = file
		ast.Walk(visitor, file)
	}
}

// typeVisitor visits all TypeSpecs, calling the given callback for each.
type valueVisitor struct {
	callback ValueCallback
	decl     *ast.GenDecl
	file     *ast.File
}

// Visit visits all TypeSpecs.
func (v *valueVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		v.decl = nil
		return v
	}

	switch valuedNode := node.(type) {
	case *ast.File:
		v.file = valuedNode
		return v
	case *ast.GenDecl:
		v.decl = valuedNode
		return v
	case *ast.ValueSpec:
		v.callback(v.file, v.decl, valuedNode)
		return nil // don't recurse
	default:
		return nil
	}
}

func extractDoc(node ast.Node, decl *ast.GenDecl) string {
	var docs *ast.CommentGroup
	switch docced := node.(type) {
	case *ast.Field:
		docs = docced.Doc
	case *ast.File:
		docs = docced.Doc
	case *ast.GenDecl:
		docs = docced.Doc
	case *ast.ValueSpec:
		docs = docced.Doc
		// type Ident expr expressions get docs attached to the decl,
		// so check for that case (missing Lparen == single line type decl)
		if docs == nil && decl.Lparen == token.NoPos {
			docs = decl.Doc
		}
	}

	if docs == nil {
		return ""
	}

	// filter out markers
	var outGroup ast.CommentGroup
	outGroup.List = make([]*ast.Comment, 0, len(docs.List))
	for _, comment := range docs.List {
		if isMarkerComment(comment.Text) {
			continue
		}
		outGroup.List = append(outGroup.List, comment)
	}

	// split lines, and re-join together as a single
	// paragraph, respecting double-newlines as
	// paragraph markers.
	outLines := strings.Split(outGroup.Text(), "\n")
	if outLines[len(outLines)-1] == "" {
		// chop off the extraneous last part
		outLines = outLines[:len(outLines)-1]
	}
	// respect double-newline meaning actual newline
	for i, line := range outLines {
		if line == "" {
			outLines[i] = "\n"
		}
	}
	return strings.Join(outLines, " ")
}

func isMarkerComment(comment string) bool {
	if comment[0:2] != "//" {
		return false
	}
	stripped := strings.TrimSpace(comment[2:])
	if len(stripped) < 1 || stripped[0] != '+' {
		return false
	}
	return true
}

type ConstInfo struct {
	// Name is the name of the type.
	Name string
	// Doc is the Godoc of the type, pre-processed to remove markers and joine
	// single newlines together.
	Doc string

	// Markers are all registered markers associated with the type.
	Markers markers.MarkerValues

	// RawDecl contains the raw GenDecl that the type was declared as part of.
	RawDecl *ast.GenDecl
	// RawSpec contains the raw Spec that declared this type.
	RawSpec *ast.ValueSpec
	// RawFile contains the file in which this type was declared.
	RawFile *ast.File
}

func EachValue(col *markers.Collector, pkg *loader.Package, cb markers.TypeCallback) error {
	// test
	markersInpkg, err := col.MarkersInPackage(pkg)
	if err != nil {
		return err
	}

	eachValue(pkg, func(file *ast.File, decl *ast.GenDecl, spec *ast.ValueSpec) {
		// var fields []markers.FieldInfo
		//if _, isStruct := spec.Type.(*ast.StructType); isStruct {
		//	log(fmt.Sprintf("in loader: is type? %v \n", decl.Tok == token.TYPE))
		//}
		if decl.Tok == token.CONST {
			log(fmt.Sprintf("in loader: is constent!!!!!! %v \n", spec.Names))
			cb(&markers.TypeInfo{
				Name:    spec.Names[0].Name,
				Markers: markersInpkg[spec],
				Doc:     extractDoc(spec, decl),
				RawDecl: decl,
				RawFile: file,
			})
		}

	})
	return nil
}

// Generate generates the artifacts
func (g Generator) Generate(ctx *genall.GenerationContext) error {
	for _, root := range ctx.Roots {

		ctx.Checker.Check(root, func(node ast.Node) bool {
			// ignore interfaces
			_, isIface := node.(*ast.InterfaceType)
			return !isIface
		})

		root.NeedTypesInfo()

		var rootStructToOverride *markers.TypeInfo
		packageTypes := map[string]*markers.TypeInfo{}

		if err := markers.EachType(ctx.Collector, root, func(info *markers.TypeInfo) {
			if info.Markers.Get(overridesTypeMarker.Name) != nil {
				if rootStructToOverride == nil {
					rootStructToOverride = info
				} else {
					root.AddError(fmt.Errorf("Marker %v should be added to only one Struct type, but was added on %v and %v",
						overridesTypeMarker.Name,
						rootStructToOverride.Name,
						info.Name,
					))
				}
			}
			packageTypes[info.RawSpec.Name.Name] = info
		}); err != nil {
			root.AddError(err)
			return nil
		}

		if err := EachValue(ctx.Collector, root, func(info *markers.TypeInfo) {
			if info.Markers.Get(overridesTypeMarker.Name) != nil {
				if rootStructToOverride == nil {
					rootStructToOverride = info
				} else {
					root.AddError(fmt.Errorf("Marker %v should be added to only one Struct type, but was added on %v and %v",
						overridesTypeMarker.Name,
						rootStructToOverride.Name,
						info.Name,
					))
				}
			}
			log(fmt.Sprintf("In EACHVALUE: %s \n", info.Name))
			packageTypes[info.Name] = info
		}); err != nil {
			root.AddError(err)
			return nil
		}

		if rootStructToOverride == nil {
			root.AddError(fmt.Errorf("Marker %v should be added to at least one Struct type",
				overridesTypeMarker.Name,
			))
			return nil
		}

		config := printer.Config{
			Tabwidth: 2,
			Mode:     printer.UseSpaces,
		}

		g.suffix = "ParentOverride"
		if g.IsForPluginOverrides {
			g.suffix = "PluginOverride"
		}
		g.rootTypeToProcess = typeToProcess{
			OverrideTypeName: g.suffix + "s",
			TypeInfo:         rootStructToOverride,
			MandatoryKey:     "",
		}

		overrides := g.process(root, packageTypes)

		fileNamePart := "parent_overrides"
		if g.IsForPluginOverrides {
			fileNamePart = "plugin_overrides"
		}

		genutils.WriteFormattedSourceFile(fileNamePart, ctx, root, func(buf *bytes.Buffer) {
			buf.WriteString(`
import (
	attributes "github.com/devfile/api/v2/pkg/attributes"
)

`)
			config.Fprint(buf, root.Fset, overrides)
			buf.WriteString(`
func (overrides ` + g.rootTypeToProcess.OverrideTypeName + `) isOverride() {}
`)
		})
	}

	return nil
}

// typeToProcess contains all required information about the how to process a given type.
// A list of `typeToProcess` instances can be returned the `createOverride` function
// since it is when processing a type that we possibly encounter new types to process.
type typeToProcess struct {
	OverrideTypeName   string
	TypeInfo           *markers.TypeInfo
	MandatoryKey       string
	DropEnumAnnotation bool
}

func (g Generator) process(root *loader.Package, packageTypes map[string]*markers.TypeInfo) []ast.Decl {
	toProcess := []typeToProcess{g.rootTypeToProcess}
	processed := orderedmap.NewOrderedMap()
	for len(toProcess) > 0 {
		nextOne := toProcess[0]
		toProcess = toProcess[1:]
		if _, isAlreadyProcessed := processed.Get(nextOne.TypeInfo.Name); isAlreadyProcessed &&
			nextOne.MandatoryKey == "" {
			log(fmt.Sprintf("skipped nextOne: %v \n", nextOne.OverrideTypeName))
			continue
		}
		newOverride, newTypesToOverride, errors := g.createOverride(nextOne, packageTypes)
		processed.Set(nextOne.TypeInfo.Name, newOverride)
		for _, err := range errors {
			root.AddError(loader.ErrFromNode(err, nextOne.TypeInfo.RawSpec))
		}
		toProcess = append(toProcess, newTypesToOverride...)
	}

	overrides := []ast.Decl{}
	for elt := processed.Front(); elt != nil; elt = elt.Next() {
		overrides = append(overrides, elt.Value.(ast.Decl))
	}
	return overrides
}

// fieldChange provides the required information about how overrides generation should handle a given field
type fieldChange struct {
	fieldInfo      markers.FieldInfo
	overrideMarker FieldOverridesInclude
}

func (g Generator) createOverride(newTypeToProcess typeToProcess, packageTypes map[string]*markers.TypeInfo) (ast.Decl, []typeToProcess, []error) {
	errors := []error{}
	var alreadyFoundType *ast.TypeSpec = nil
	fieldChanges := map[token.Pos]fieldChange{}

	typeToOverride := newTypeToProcess.TypeInfo
	if typeToOverride.Fields != nil {
		for _, field := range typeToOverride.Fields {
			fieldPos := field.RawField.Pos()
			if !fieldPos.IsValid() {
				errors = append(errors,
					fmt.Errorf("Field %v in type %v doesn't have a valid position in the source file",
						field.Name,
						typeToOverride.Name,
					))
				continue
			}
			overridesMarker := FieldOverridesInclude{}
			if markerEntry := field.Markers.Get(overridesFieldMarker.Name); markerEntry != nil {
				overridesMarker = markerEntry.(FieldOverridesInclude)
			}

			fieldChanges[fieldPos] = fieldChange{
				fieldInfo:      field,
				overrideMarker: overridesMarker,
			}
		}
	}

	overrideGenDecl := astcopy.GenDecl(typeToOverride.RawDecl)
	if typeToOverride.Markers.Get(overridesTypeMarker.Name) != nil {
		overrideGenDecl.Doc = updateComments(overrideGenDecl, overrideGenDecl.Doc, `.*`, ` *\+`+overridesTypeMarker.Name+` *`)
	}
	if newTypeToProcess.DropEnumAnnotation {
		overrideGenDecl.Doc = updateComments(
			overrideGenDecl, overrideGenDecl.Doc,
			`.*`,
			` *`+regexp.QuoteMeta("+kubebuilder:validation:Enum=")+`.*`,
		)
	}

	overrideGenDecl.Doc = updateComments(
		overrideGenDecl, overrideGenDecl.Doc,
		`.*`,
		` *`+regexp.QuoteMeta("+devfile:jsonschema:generate")+` *`,
	)

	if newTypeToProcess == g.rootTypeToProcess {
		overrideGenDecl.Doc = updateComments(
			overrideGenDecl, overrideGenDecl.Doc,
			`.*`,
			``,
			"+devfile:jsonschema:generate",
		)
	}

	moreTypesToAdd := []typeToProcess{}
	overrideGenDecl = astutil.Apply(overrideGenDecl,
		func(cursor *astutil.Cursor) bool {
			processFieldType := func(ident *ast.Ident) *typeToProcess {
				typeToOverride, existsInPackage := packageTypes[ident.Name]
				if !existsInPackage {
					return nil
				}
				ident.Name = ident.Name + g.suffix
				return &typeToProcess{
					OverrideTypeName: ident.Name,
					TypeInfo:         typeToOverride,
				}
			}
			if valueSpec, isValueSpec := cursor.Node().(*ast.ValueSpec); isValueSpec {
				log("VALUE SPEC!!!!!!  ")
				switch fieldType := valueSpec.Type.(type) {
				case *ast.Ident:

					log(fmt.Sprintf("FIELD NAME %s", fieldType.Name))
					fieldTypeToProcess := processFieldType(fieldType)
					if fieldTypeToProcess != nil {
						moreTypesToAdd = append(moreTypesToAdd, *fieldTypeToProcess)
					}
				}
			}
			if typeSpec, isTypeSpec := cursor.Node().(*ast.TypeSpec); isTypeSpec {
				log(fmt.Sprintf("Name: %s\n",typeSpec.Name.Name))
				if alreadyFoundType != nil {
					errors = append(errors,
						fmt.Errorf("types %v and %v are defined in the same type definition - please avoid defining several types in the same type definition",
							alreadyFoundType.Name,
							typeSpec.Name,
						))
					return false
				}
				typeSpec.Name.Name = newTypeToProcess.OverrideTypeName
			}
			if astField, isField := cursor.Node().(*ast.Field); isField {
				if newTypeToProcess == g.rootTypeToProcess &&
					cursor.Index() == 0 {
					cursor.InsertBefore(&ast.Field{
						Type: &ast.Ident{Name: "OverridesBase"},
						Tag:  &ast.BasicLit{Kind: token.STRING, Value: "`json:\",inline\"`"},
					})
				}

				fieldChange := fieldChanges[cursor.Node().Pos()]
				field := fieldChange.fieldInfo

				overridesMarker := fieldChange.overrideMarker

				shouldSkip := func(overridesMarker FieldOverridesInclude) bool {
					if overridesMarker.Omit ||
						(overridesMarker.OmitInPlugin && g.IsForPluginOverrides) {
						return true
					}
					return false
				}

				if shouldSkip(overridesMarker) {
					cursor.Delete()
					return true
				}

				if overridesMarker.Description != "" {
					astField.Doc = updateComments(
						astField, astField.Doc,
						` *\+[^ ]+.*`,
						` *\+`+overridesFieldMarker.Name+`.*`,
						overridesMarker.Description,
						"Overriding is done according to K8S strategic merge patch standard rules.",
					)
				}

				if field.Name != newTypeToProcess.MandatoryKey {
					// Make the field optional by default, unless typeToProcess contains a MandatoryKey nonempty field
					jsonTag := field.Tag.Get("json")
					if jsonTag != "" &&
						jsonTag != "-" &&
						!strings.Contains(jsonTag, ",inline") &&
						!strings.Contains(jsonTag, ",omitempty") {
						newJSONTag := jsonTag + ",omitempty"
						astField.Tag.Value = strings.Replace(astField.Tag.Value, `json:"`+jsonTag+`"`, `json:"`+newJSONTag+`"`, 1)
						astField.Doc = updateComments(
							astField, astField.Doc,
							`.*`,
							` *`+regexp.QuoteMeta("+optional")+`.*`,
							` +optional`,
						)
					}
				}

				// Remove the `default` directives for overrides, since it doesn't make sense.
				astField.Doc = updateComments(
					astField, astField.Doc,
					`.*`,
					` *`+regexp.QuoteMeta("+kubebuilder:default")+` *=.*`,
				)


				var fieldTypeToProcess *typeToProcess
				switch fieldType := astField.Type.(type) {
				case *ast.ArrayType:
					switch elementType := fieldType.Elt.(type) {
					case *ast.Ident:
						fieldTypeToProcess = processFieldType(elementType)
						if fieldTypeToProcess != nil {
							fieldTypeToProcess.MandatoryKey = strings.Title(genutils.GetPatchMergeKey(&field))
						}
					}
				case *ast.Ident:
					fieldTypeToProcess = processFieldType(fieldType)
					if field.Markers.Get(genutils.UnionDiscriminatorMarker.Name) != nil {
						enumValues := []string{}
						for _, f := range typeToOverride.Fields {
							pos := f.RawField.Pos()
							fieldChange := fieldChanges[f.RawField.Pos()]
							if pos != cursor.Node().Pos() &&
								!shouldSkip(fieldChange.overrideMarker) {
								enumValues = append(enumValues, fieldChange.fieldInfo.Name)
							}
						}
						kubebuilderAnnotation := "+kubebuilder:validation:Enum=" + strings.Join(enumValues, ";")
						astField.Doc = updateComments(
							astField, astField.Doc,
							`.*`,
							` *`+regexp.QuoteMeta("+kubebuilder:validation:Enum=")+`.*`,
							kubebuilderAnnotation)
						if fieldTypeToProcess != nil {
							fieldTypeToProcess.DropEnumAnnotation = true
						}
					}
				case *ast.StarExpr:
					switch elementType := fieldType.X.(type) {
					case *ast.Ident:
						fieldTypeToProcess = processFieldType(elementType)
					}
				case *ast.MapType:
					switch elementType := fieldType.Key.(type) {
					case *ast.Ident:
						fieldTypeToProcess = processFieldType(elementType)
					}
					switch elementType := fieldType.Value.(type) {
					case *ast.Ident:
						fieldTypeToProcess = processFieldType(elementType)
					}
				default:
				}

				if fieldTypeToProcess != nil {
					moreTypesToAdd = append(moreTypesToAdd, *fieldTypeToProcess)
				}
			}
			return true
		},
		func(*astutil.Cursor) bool { return true },
	).(*ast.GenDecl)

	return overrideGenDecl, moreTypesToAdd, errors
}

// writeFormatted outputs the given code, after gofmt-ing it.  If we couldn't gofmt,
// we write the unformatted code for debugging purposes.
func (g Generator) writeOut(ctx *genall.GenerationContext, root *loader.Package, outBytes []byte) {
	fileToWrite := "zz_generated.parent_overrides.go"
	if g.IsForPluginOverrides {
		fileToWrite = "zz_generated.plugin_overrides.go"
	}
	outputFile, err := ctx.Open(root, fileToWrite)
	if err != nil {
		root.AddError(err)
		return
	}
	defer outputFile.Close()
	n, err := outputFile.Write(outBytes)
	if err != nil {
		root.AddError(err)
		return
	}
	if n < len(outBytes) {
		root.AddError(io.ErrShortWrite)
	}
}

func log( msg string) {
	fileToWrite := "test_log.txt"

		f, _ := os.OpenFile(fileToWrite, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer f.Close()
		data := []byte(msg)
		f.Write(data)

}

// updateComments defines, through regexps, which comment lines should be kept and which should be dropped,
// It also provides additional comment lines that will be *prepended* to the existing comment lines.
// In both regexps and additional lines, the comment prefix `//` should be omitted.
func updateComments(commentedNode ast.Node, commentGroup *ast.CommentGroup, keepRegexp string, dropRegexp string, additionalLines ...string) *ast.CommentGroup {
	if commentGroup == nil {
		commentGroup = &ast.CommentGroup{}
	}
	commentsToKeep := []*ast.Comment{}
	for _, comment := range commentGroup.List {
		if keep, _ := regexp.MatchString(`^ *//`+keepRegexp+`$`, comment.Text); keep {
			if drop, _ := regexp.MatchString(`^ *//`+dropRegexp+`$`, comment.Text); !drop {
				comment.Slash = token.NoPos
				commentsToKeep = append(commentsToKeep, comment)
			}
		}
	}
	commentGroup.List = []*ast.Comment{}
	for _, line := range additionalLines {
		commentGroup.List = append(commentGroup.List, &ast.Comment{Text: "// " + line})
	}
	commentGroup.List = append(commentGroup.List, commentsToKeep...)
	if len(commentGroup.List) > 0 {
		commentGroup.List[0].Slash = commentedNode.Pos() - 1
	}
	return commentGroup
}
