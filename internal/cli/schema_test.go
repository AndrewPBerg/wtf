package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
)

func schemaRegistry() *cobra.Command {
	return New(core.New("", "", io.Discard), io.Discard, io.Discard).Root
}

func schemaDocument(t *testing.T, root *cobra.Command) map[string]any {
	t.Helper()
	data, err := ConfigurationSchema(root)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func schemaAt(t *testing.T, document map[string]any, path ...string) map[string]any {
	t.Helper()
	var value any = document
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("non-object on schema path %v", path)
		}
		value = object[key]
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("missing schema path %v", path)
	}
	if ref, ok := object["$ref"].(string); ok {
		pointer, err := url.PathUnescape(strings.TrimPrefix(ref, "#/"))
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(pointer, "/")
		for i, key := range parts {
			parts[i] = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
		}
		return schemaAt(t, document, parts...)
	}
	return object
}

func TestConfigurationSchemaArtifactIsCurrent(t *testing.T) {
	generated, err := ConfigurationSchema(schemaRegistry())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join("..", "..", ".wtf", "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, stored) {
		t.Fatal("schema is stale: run GOMAXPROCS=1 go generate ./internal/cli")
	}
}

func TestConfigurationSchemaDocumentsTypesAliasesAndNestedCommands(t *testing.T) {
	doc := schemaDocument(t, schemaRegistry())
	if doc["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Fatal("missing portable schema dialect")
	}
	pr := schemaAt(t, doc, "properties", "commands", "definitions", "list", "definitions", "pr")
	if pr["type"] != "boolean" || pr["default"] != false || !strings.Contains(pr["description"].(string), "--pr / -p") {
		t.Fatalf("PR flag metadata missing: %+v", pr)
	}
	alias := schemaAt(t, doc, "properties", "commands", "properties", "ls")
	canonical := schemaAt(t, doc, "properties", "commands", "properties", "list")
	a, _ := json.Marshal(alias)
	b, _ := json.Marshal(canonical)
	if !bytes.Equal(a, b) {
		t.Fatal("alias schema differs")
	}
	port := schemaAt(t, doc, "properties", "commands", "definitions", "ports claim", "definitions", "port")
	if port["type"] != "integer" {
		t.Fatalf("nested command schema missing: %+v", port)
	}
	task := schemaAt(t, doc, "properties", "tasks", "additionalProperties")
	if task["additionalProperties"] != false {
		t.Fatal("unknown task fields would be accepted")
	}
	command := schemaAt(t, doc, "properties", "tasks", "additionalProperties", "properties", "command")
	if command["minItems"] != float64(1) {
		t.Fatal("empty command argv would be accepted")
	}
	for _, name := range []string{"LS", "LiSt", "hook RUN"} {
		patterns := schemaAt(t, doc, "properties", "commands")["patternProperties"].(map[string]any)
		matched := false
		for pattern := range patterns {
			if regexp.MustCompile(pattern).MatchString(name) {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("case-insensitive command missing: %s", name)
		}
	}
}

func TestConfigurationSchemaIncludesFutureCommandWithoutRunningIt(t *testing.T) {
	root := schemaRegistry()
	group := &cobra.Command{Use: "future", Short: "Future group", Aliases: []string{"f"}}
	child := &cobra.Command{Use: "inspect", Short: "Future inspection", Aliases: []string{"i"}, Run: func(*cobra.Command, []string) { t.Fatal("schema generation executed a command") }}
	child.Flags().BoolP("feature", "e", true, "Show the future feature")
	group.AddCommand(child)
	root.AddCommand(group)
	doc := schemaDocument(t, root)
	flag := schemaAt(t, doc, "properties", "commands", "definitions", "future inspect", "definitions", "feature")
	if flag["type"] != "boolean" || flag["default"] != true || !strings.Contains(flag["description"].(string), "future feature") {
		t.Fatalf("future flag not documented: %+v", flag)
	}
	schemaAt(t, doc, "properties", "commands", "properties", "f i")
	child.Flags().StringSlice("unsupported", nil, "Unsupported config binding")
	if _, err := ConfigurationSchema(root); err == nil {
		t.Fatal("unsupported binding silently generated a misleading schema")
	}
}

func TestConfigurationSchemaReferencesResolveAndDirectivePreservesParsing(t *testing.T) {
	document := schemaDocument(t, schemaRegistry())
	var inspect func(any)
	inspect = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				if strings.Contains(ref, " ") {
					t.Fatalf("reference contains unescaped spaces: %s", ref)
				}
				parsed, err := url.Parse(ref)
				if err != nil || parsed.Fragment == "" {
					t.Fatalf("invalid reference %q: %v", ref, err)
				}
				pointer := strings.TrimPrefix(parsed.Fragment, "/")
				parts := strings.Split(pointer, "/")
				for i, key := range parts {
					parts[i] = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
				}
				schemaAt(t, document, parts...)
			}
			for _, child := range value {
				inspect(child)
			}
		case []any:
			for _, child := range value {
				inspect(child)
			}
		}
	}
	inspect(document)
	config, err := core.LoadConfig(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("schema directive broke config parsing: %v", err)
	}
	if config.Version != 1 {
		t.Fatalf("unexpected config version: %d", config.Version)
	}
}
