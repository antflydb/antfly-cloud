package main

import (
	"bytes"
	"encoding/json"
	"github.com/spf13/cobra"
	"strings"
	"testing"
)

func TestManifestPreservesOpaqueNumbersForAllInputForms(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"JSON stdin", `{"version":1,"sources":[{"key":"docs","connection":"c","table":"t","config":{"integer":18446744073709551617,"decimal":0.10000000000000001}}]}`},
		{"YAML stdin", "version: 1\nsources:\n  - key: docs\n    connection: c\n    table: t\n    config: {integer: 18446744073709551617, decimal: 0.10000000000000001}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader(test.body))
			manifest, err := readSourceManifest(cmd, "-")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(manifest.Sources[0].Config)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte("18446744073709551617")) || !bytes.Contains(raw, []byte("0.10000000000000001")) {
				t.Fatalf("rounded config: %s", raw)
			}
		})
	}
}
func TestManifestYAMLAnchorsAndExactNumbers(t *testing.T) {
	raw, err := sourceManifestYAMLToJSON([]byte("defaults: &defaults {count: 18446744073709551617, ratio: .10000000000000001}\nconfig: {<<: *defaults, count: 18446744073709551618}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"count":18446744073709551618`)) || !bytes.Contains(raw, []byte(`"ratio":0.10000000000000001`)) {
		t.Fatalf("merge or precision lost: %s", raw)
	}
	for _, body := range []string{"a: 1\na: 2\n", "1: value\n", "a: .inf\n", "a: !!float ''\n", "a: &a [*a]\n", "a: 1\n---\na: 2\n"} {
		if _, err := sourceManifestYAMLToJSON([]byte(body)); err == nil {
			t.Fatalf("accepted invalid YAML: %q", body)
		}
	}
}
func TestManifestYAMLBoundsAliasExpansion(t *testing.T) {
	input := "a: &a [" + strings.Repeat("x,", 100) + "]\n"
	previous := "a"
	for _, name := range []string{"b", "c", "d", "e"} {
		input += name + ": &" + name + " [" + strings.Repeat("*"+previous+",", 10) + "]\n"
		previous = name
	}
	if _, err := sourceManifestYAMLToJSON([]byte(input)); err == nil {
		t.Fatal("unbounded alias expansion accepted")
	}
}

func TestManifestYAMLNumbersOutsideFloatRange(t *testing.T) {
	raw, err := sourceManifestYAMLToJSON([]byte("large: 1e400\nsmall: 1e-400\nquoted: '1e400'\nexplicit: !!str 1e400\n"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value["large"] != json.Number("1e400") || value["small"] != json.Number("1e-400") || value["quoted"] != "1e400" || value["explicit"] != "1e400" {
		t.Fatalf("numeric/string types changed: %#v", value)
	}
}
