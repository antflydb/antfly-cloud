package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
)

func TestManifestEmptyArraysAndOmittedNameConverge(t *testing.T) {
	f := newManifestTestFixture(t)
	input := manifestTestYAML + "    search_version: 2\n    streams: []\n    artifacts: []\n"
	named := input + "    name: Engineering docs\n"
	if output, err := f.execute(true, named, "apply", "--file", "-"); err != nil {
		t.Fatalf("initial apply: %v: %s", err, output)
	}
	manifestTestAssertWrites(t, f, 1)
	for _, document := range []string{named, input} {
		output, err := f.execute(true, document, "apply", "--file", "-")
		if err != nil {
			t.Fatalf("reapply: %v: %s", err, output)
		}
		var result struct {
			Results []commands.ApplyResult `json:"results"`
		}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Results) != 1 || result.Results[0].Outcome != "unchanged" {
			t.Fatalf("reapply: %s", output)
		}
		manifestTestAssertWrites(t, f, 1)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.sources[manifestTestExpectedID(manifestTestKey)].Name; got != "Engineering docs" {
		t.Fatalf("display name changed to %q", got)
	}
}

func TestManifestApplyRejectsInvalidSecondDeclarationBeforeWrites(t *testing.T) {
	f := newManifestTestFixture(t)
	input := manifestTestYAML + `  - key: invalid
    connection: company-google
    table: docs
    config: {resource: drive}
    streams: [{}]
`
	output, err := f.execute(true, input, "apply", "--file", "-")
	if errorExitCode(err) != 2 || !strings.Contains(err.Error(), "sources[1] (invalid)") || output != "" {
		t.Fatalf("invalid declaration: output=%q error=%v", output, err)
	}
	manifestTestAssertWrites(t, f, 0)
}

func TestManifestApplyErrorsRetainScriptCategories(t *testing.T) {
	for _, tc := range []struct {
		status, exit   int
		code, category string
	}{
		{400, 2, "invalid_source", "invalid"},
		{401, 3, "expired", "unauthorized"},
		{403, 3, "denied", "forbidden"},
		{404, 4, "source_not_found", "not_found"},
		{412, 5, "source_already_exists", "conflict"},
		{501, 6, "revision_preconditions_unsupported", "unsupported"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			f := newManifestTestFixture(t)
			f.writeErrorStatus, f.writeErrorCode = tc.status, tc.code
			output, err := f.execute(true, manifestTestYAML, "apply", "--file", "-")
			if errorExitCode(err) != tc.exit {
				t.Fatalf("exit=%d error=%v", errorExitCode(err), err)
			}
			var result struct {
				Results []commands.ApplyResult `json:"results"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Results) != 1 {
				t.Fatalf("results: %s", output)
			}
			got := result.Results[0]
			if got.ErrorCode != tc.category || got.HTTPStatus != tc.status || got.ServerCode != tc.code {
				t.Fatalf("lost per-source error details: %s", output)
			}
		})
	}
}
