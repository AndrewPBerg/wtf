package cli

//go:generate go run -p 1 ../../cmd/wtf-schema -output testdata/config-schema.json

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type schemaObject map[string]any

// ConfigurationSchema reads definitions only: no Execute, config loading, hooks or network.
func ConfigurationSchema(root *cobra.Command) ([]byte, error) {
	if root == nil {
		return nil, fmt.Errorf("command registry is required")
	}
	commands, err := commandSchemas(root)
	if err != nil {
		return nil, err
	}
	schema, err := tomlTypeSchema(reflect.TypeOf(core.Config{}), commands)
	if err != nil {
		return nil, err
	}
	schema["$schema"] = "http://json-schema.org/draft-07/schema#"
	schema["$id"] = "urn:wtf:config:1"
	schema["title"] = "WTF v1 configuration"
	schema["$comment"] = "Generated from Go TOML types and the Cobra command registry. Run go generate ./internal/cli; do not edit manually."
	schema["description"] = "Editor assistance for WTF configuration. Runtime validation remains authoritative; schema default annotations do not apply inherited settings."
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func tomlTypeSchema(t reflect.Type, commands schemaObject) (schemaObject, error) {
	switch t.Kind() {
	case reflect.String:
		return schemaObject{"type": "string"}, nil
	case reflect.Bool:
		return schemaObject{"type": "boolean"}, nil
	case reflect.Int:
		return schemaObject{"type": "integer"}, nil
	case reflect.Slice:
		item, err := tomlTypeSchema(t.Elem(), commands)
		if err != nil {
			return nil, err
		}
		return schemaObject{"type": "array", "items": item}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported TOML map key: %s", t)
		}
		item, err := tomlTypeSchema(t.Elem(), commands)
		if err != nil {
			return nil, err
		}
		return schemaObject{"type": "object", "additionalProperties": item}, nil
	case reflect.Struct:
		properties := schemaObject{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("toml"), ",")[0]
			if field.PkgPath != "" || name == "-" {
				continue
			}
			if name == "" || field.Tag.Get("schema") == "" {
				return nil, fmt.Errorf("%s.%s needs TOML and schema documentation tags", t.Name(), field.Name)
			}
			var child schemaObject
			var err error
			if t == reflect.TypeOf(core.Config{}) && field.Name == "Commands" {
				child = commands
			} else {
				child, err = tomlTypeSchema(field.Type, commands)
			}
			if err != nil {
				return nil, err
			}
			child["description"] = field.Tag.Get("schema")
			for _, pair := range [][2]string{{"schema-const", "const"}, {"schema-default", "default"}, {"schema-minimum", "minimum"}, {"schema-maximum", "maximum"}, {"schema-min-items", "minItems"}} {
				if text := field.Tag.Get(pair[0]); text != "" {
					var value any
					if err = json.Unmarshal([]byte(text), &value); err != nil {
						return nil, fmt.Errorf("invalid %s on %s.%s: %w", pair[0], t.Name(), field.Name, err)
					}
					child[pair[1]] = value
				}
			}
			properties[name] = child
			if field.Tag.Get("schema-required") == "true" {
				required = append(required, name)
			}
		}
		result := schemaObject{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			result["required"] = required
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported TOML schema type: %s", t)
	}
}

// Explicit lowercase properties provide completion; patterns accept mixed-case spellings.
func casePattern(name string) string {
	var pattern strings.Builder
	pattern.WriteByte('^')
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' {
			pattern.WriteRune('[')
			pattern.WriteRune(r)
			pattern.WriteRune(r - 'a' + 'A')
			pattern.WriteRune(']')
		} else {
			pattern.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	pattern.WriteByte('$')
	return pattern.String()
}

func commandSchemas(root *cobra.Command) (schemaObject, error) {
	properties, patterns := schemaObject{}, schemaObject{}
	definitions := schemaObject{}
	var walk func(*cobra.Command, []string) error
	walk = func(command *cobra.Command, variants []string) error {
		flags := map[string]*pflag.Flag{}
		add := func(flag *pflag.Flag) {
			if !flag.Hidden && flag.Name != "help" {
				if _, exists := flags[flag.Name]; !exists {
					flags[flag.Name] = flag
				}
			}
		}
		command.Flags().VisitAll(add)
		for parent := command; parent != nil; parent = parent.Parent() {
			parent.PersistentFlags().VisitAll(add)
		}
		flagProps, flagPatterns := schemaObject{}, schemaObject{}
		flagDefinitions := schemaObject{}
		canonical := strings.TrimPrefix(command.CommandPath(), root.Name()+" ")
		for name, flag := range flags {
			if flag.Usage == "" {
				return fmt.Errorf("%s --%s needs help text", command.CommandPath(), name)
			}
			field := schemaObject{}
			var defaultValue any
			var err error
			switch flag.Value.Type() {
			case "bool":
				field["type"] = "boolean"
				defaultValue, err = strconv.ParseBool(flag.DefValue)
			case "int":
				field["type"] = "integer"
				defaultValue, err = strconv.ParseInt(flag.DefValue, 10, 64)
			case "string":
				field["type"] = "string"
				defaultValue = flag.DefValue
			default:
				return fmt.Errorf("%s --%s: config binding and schema support are needed for %s", command.CommandPath(), name, flag.Value.Type())
			}
			if err != nil {
				return fmt.Errorf("%s --%s default: %w", command.CommandPath(), name, err)
			}
			cliName := "--" + name
			if flag.Shorthand != "" {
				cliName += " / -" + flag.Shorthand
			}
			field["default"] = defaultValue
			field["description"] = flag.Usage + " CLI: " + cliName + ". Explicit CLI flags override inherited TOML defaults."
			flagDefinitions[name] = field
			ref := schemaObject{"$ref": "#/properties/commands/definitions/" + schemaPointer(canonical) + "/definitions/" + schemaPointer(name)}
			flagProps[name] = ref
			flagPatterns[casePattern(name)] = ref
		}
		if command != root && !command.Hidden && command.Name() != "help" {
			description := command.Long
			if description == "" {
				description = command.Short
			}
			object := schemaObject{"type": "object", "description": description, "properties": flagProps, "patternProperties": flagPatterns, "definitions": flagDefinitions, "additionalProperties": false}
			definitions[canonical] = object
			ref := schemaObject{"$ref": "#/properties/commands/definitions/" + schemaPointer(canonical)}
			for _, name := range variants {
				if _, exists := properties[name]; exists {
					return fmt.Errorf("ambiguous configured command alias: %s", name)
				}
				properties[name] = ref
				patterns[casePattern(name)] = ref
			}
		}
		for _, child := range command.Commands() {
			if child.Hidden || child.Name() == "help" {
				continue
			}
			names := append([]string{child.Name()}, child.Aliases...)
			paths := []string{}
			for _, parent := range variants {
				for _, name := range names {
					paths = append(paths, strings.TrimSpace(parent+" "+name))
				}
			}
			if err := walk(child, paths); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, []string{""}); err != nil {
		return nil, err
	}
	return schemaObject{"type": "object", "properties": properties, "patternProperties": patterns, "definitions": definitions, "additionalProperties": false}, nil
}

func schemaPointer(key string) string {
	return url.PathEscape(strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1"))
}
