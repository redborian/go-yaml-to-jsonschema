package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"
)

const defPrefix = "#/definitions/"

// Checks whether the typeName represents a simple json type

// Gets the type name of the array
func getTypeNameOfArray(arrayType *ast.ArrayType) string {
	switch arrayType.Elt.(type) {
	case *ast.Ident:
		identifier := arrayType.Elt.(*ast.Ident)
		return identifier.Name
	case *ast.StarExpr:
		starType := arrayType.Elt.(*ast.StarExpr)
		identifier := starType.X.(*ast.Ident)
		return identifier.Name
	}
	panic("undefined type")
}

// Removes a character by replacing it with a space
func removeChar(str string, removedStr string) string {
	return strings.Replace(str, removedStr, " ", -1)
}

// This is a hacky function that does the one job of
// extracting the tag values in the structs
// Example struct:
// type MyType struct {
//   MyField string `yaml:"myField,omitempty"`
// }
//
// From the above example struct, we need to extract
// and return this: ("myField", "omitempty")
func extractFromTag(tag *ast.BasicLit) (string, string) {
	tagValue := tag.Value
	if tagValue == "" {
		log.Panic("Tag value is empty")
	}

	// return yamlFieldValue, yamlOptionValue
	tagValue = removeChar(tagValue, "`")
	tagValue = removeChar(tagValue, `"`)
	tagValue = strings.TrimSpace(tagValue)

	var yamlTagContent string
	fmt.Sscanf(tagValue, `json: %s`, &yamlTagContent)

	if strings.Contains(yamlTagContent, ",") {
		splitContent := strings.Split(yamlTagContent, ",")
		return splitContent[0], splitContent[1]
	}
	return yamlTagContent, ""
}

// Gets the schema definition link of a resource
func getDefLink(resourceName string) string {
	return defPrefix + resourceName
}

// Parses array type and returns its corresponding
// schema definition.
func parseArrayType(arrayType *ast.ArrayType) *Definition {
	definition := new(Definition)
	typeNameOfArray := getTypeNameOfArray(arrayType)

	definition.Items = new(Definition)
	if isSimpleType(typeNameOfArray) {
		definition.Items.Type = jsonifyType(typeNameOfArray)
	} else {
		definition.Items.Ref = getDefLink(typeNameOfArray)
	}
	definition.Type = "array"

	return definition
}

// Merges the properties from the 'rhsDef' to the 'lhsDef'.
// Also transfers the description as well.
func mergeDefinitions(lhsDef *Definition, rhsDef *Definition) {
	// At this point, both defs will not have any 'AnyOf' defs.
	// 1. Add all the properties from rhsDef to lhsDef
	if lhsDef.Properties == nil {
		lhsDef.Properties = make(map[string]*Definition)
	}
	for propKey, propValue := range rhsDef.Properties {
		lhsDef.Properties[propKey] = propValue
	}
	// 2. Transfer the description
	if len(lhsDef.Description) == 0 {
		lhsDef.Description = rhsDef.Description
	}
}

// Gets the resource name from definitions url.
// Eg, returns 'TypeName' from '#/definitions/TypeName'
func getNameFromURL(url string) string {
	slice := strings.Split(url, "/")
	return slice[len(slice)-1]
}

// Parses a struct type and returns its corresponding
// schema definition.
func parseStructType(structType *ast.StructType, typeName string, typeDescription string) (*Definition, map[string][]string) {
	definition := &Definition{}
	definition.Description = typeDescription
	definition.Properties = make(map[string]*Definition)
	definition.Required = []string{}
	inlineDefinitions := []*Definition{}
	externalTypes := make(map[string][]string)

	for _, field := range structType.Fields.List {
		property := new(Definition)
		yamlFieldName, option := extractFromTag(field.Tag)

		// If the 'inline' option is enabled, we need to merge
		// the type with its parent definition. We do it with
		// 'anyOf' json schema property.
		if option == "inline" {
			var typeName string
			switch field.Type.(type) {
			case *ast.Ident:
				typeName = field.Type.(*ast.Ident).String()
			case *ast.StarExpr:
				typeName = field.Type.(*ast.StarExpr).X.(*ast.Ident).String()
			}
			inlinedDef := new(Definition)
			inlinedDef.Ref = getDefLink(typeName)
			inlineDefinitions = append(inlineDefinitions, inlinedDef)
			continue
		}
		// if 'omitempty' is not present, then the field is required
		if option != "omitempty" {
			definition.Required = append(definition.Required, yamlFieldName)
		}

		switch field.Type.(type) {
		case *ast.Ident:
			typeName := field.Type.(*ast.Ident).String()
			if isSimpleType(typeName) {
				property.Type = jsonifyType(typeName)
			} else {
				property.Ref = getDefLink(typeName)
			}
		case *ast.ArrayType:
			arrayType := field.Type.(*ast.ArrayType)
			property = parseArrayType(arrayType)
		case *ast.MapType:
			mapType := field.Type.(*ast.MapType)
			switch mapType.Value.(type) {
			case *ast.Ident:
				valueType := mapType.Value.(*ast.Ident)
				property.AdditionalProperties = new(Definition)

				if isSimpleType(valueType.Name) {
					property.AdditionalProperties.Type = valueType.Name
				} else {
					property.AdditionalProperties.Ref = getDefLink(valueType.Name)
				}
			case *ast.InterfaceType:
				// No op
			}
			property.Type = "object"
		case *ast.SelectorExpr:
			selectorType := field.Type.(*ast.SelectorExpr)
			prefix := selectorType.X.(*ast.Ident).Name
			typeName := selectorType.Sel.Name

			property.Ref = getDefLink(typeName)
			// Collect external references
			externalTypes[prefix] = append(externalTypes[prefix], typeName)
		case *ast.StarExpr:
			starExpr := field.Type.(*ast.StarExpr)
			// starType := starExpr.X.(*ast.Ident)
			// typeName := starType.Name

			// if isSimpleType(typeName) {
			// 	property.Type = jsonifyType(typeName)
			// } else {
			// 	property.Ref = getDefLink(typeName)
			// }
			switch starExpr.X.(type) {
			case *ast.Ident:
				starType := starExpr.X.(*ast.Ident)
				typeName := starType.Name

				if isSimpleType(typeName) {
					property.Type = jsonifyType(typeName)
				} else {
					property.Ref = getDefLink(typeName)
				}
			case *ast.SelectorExpr:
				selectorType := starExpr.X.(*ast.SelectorExpr)
				// println(selectorType)
				fmt.Printf("%+v\n", selectorType)
				fmt.Printf("TODO")
				// panic("Stop Execution")
			}
		}
		// Set the common properties here as the cases might
		// overwrite 'property' pointer.
		property.Description = field.Doc.Text()

		definition.Properties[yamlFieldName] = property
	}

	if len(inlineDefinitions) == 0 {
		// There are no inlined definitions
		return definition, externalTypes
	}

	// There are inlined definitions; we need to set
	// the "anyOf" property of a new parent node and attach
	// the inline definitions, along with the currently
	// parsed definition
	parentDefinition := new(Definition)
	parentDefinition.AllOf = inlineDefinitions

	if len(definition.Properties) != 0 {
		parentDefinition.AllOf = append(inlineDefinitions, definition)
	}

	return parentDefinition, externalTypes
}

func getReachableTypes(startingTypes []string) []string {
	return []string{}
}

func parseTypesInFile(filePath string, referencedTypes []string, containsAllTypes bool) Definitions {
	// Open the input go file and parse the Abstract Syntax Tree
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	definitions := make(Definitions)
	externalTypes := make(map[string][]string)
	for _, i := range node.Decls {
		declaration, ok := i.(*ast.GenDecl)
		if !ok {
			continue
		}

		for _, spec := range declaration.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeName := typeSpec.Name.Name
			typeDescription := declaration.Doc.Text()

			fmt.Println("Generating schema definition for type:", typeName)

			// Currently schema generation only takes Structs
			// and Array types into account.
			switch typeSpec.Type.(type) {
			case *ast.ArrayType:
				arrayType := typeSpec.Type.(*ast.ArrayType)
				parsedArrayDef := parseArrayType(arrayType)

				definitions[typeName] = parsedArrayDef
				// collectExternalTypeFromArray(externalTypes, arrayType)
			case *ast.StructType:
				structType := typeSpec.Type.(*ast.StructType)
				parsedStructDef, referencedTypes := parseStructType(structType, typeName, typeDescription)
				mergeMaps(externalTypes, referencedTypes)

				definitions[typeName] = parsedStructDef
			}
		}
	}

	// Prune unreferenced types
	if !containsAllTypes {
		allReachableTypes := getReachableTypes(referencedTypes)
		for key := range definitions {
			if !arrayContains(allReachableTypes, key) {
				delete(definitions, key)
			}
		}
	}

	// Expand external references
	// Assume no cyclic references across files
	// fmt.Println("%+v", node.Imports)
	// debugPrint(node.Imports)
	importPaths := make(map[string]string)
	for _, importItem := range node.Imports {
		if importItem.Name != nil {
			importPaths[importItem.Name.Name] = importItem.Path.Value
		}
	}
	debugPrint(externalTypes)
	debugPrint(importPaths)

	return definitions
}

func main() {
	inputPath := flag.String("input-file", "", "Input go file path")
	outputPath := flag.String("output-file", "", "Output schema json path")
	removeAllOfs := flag.Bool("remove-allof", false, "Flattens the json schema by removing \"allOf\"s")

	flag.Parse()

	if len(*inputPath) == 0 || len(*outputPath) == 0 {
		log.Panic("Both input path and output paths need to be set")
	}

	schema := Schema{
		Definition:  &Definition{},
		Definitions: make(map[string]*Definition)}
	schema.Type = "object"
	// fmt.Println("%+v", node.Imports)
	// b, err3 := json.Marshal(node.Imports)
	// if err3 != nil {
	// 	panic(err)
	// }
	// fmt.Println(string(b))

	schema.Definitions = parseTypesInFile(*inputPath, []string{}, true)

	if *removeAllOfs {
		fmt.Println("Flattening the schema by removing \"anyOf\" nodes")
		flattenSchema(&schema)
	}

	out, err := os.Create(*outputPath)
	if err != nil {
		log.Panic(err)
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	err = enc.Encode(schema)
	if err2 := out.Close(); err == nil {
		err = err2
	}
	if err != nil {
		log.Panic(err)
	}
}
