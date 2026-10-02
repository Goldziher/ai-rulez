package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeConfigDocs_HandBuiltInputs(t *testing.T) {
	tests := []struct {
		name    string
		shared  map[string]any
		local   map[string]any
		want    string
		wantErr string
	}{
		{
			name:   "json style values and typed slices",
			shared: map[string]any{"version": "4", "mcp_servers": []map[string]any{{"name": "a", "args": []string{"x"}}}},
			local:  map[string]any{"mcp_servers": []any{map[string]any{"name": "a", "timeout": float64(5)}}},
			want:   `{"version":"4","mcp_servers":[{"name":"a","args":["x"],"timeout":5}]}`,
		},
		{
			name:   "yaml v2 style maps are converted",
			shared: map[string]any{"header": map[any]any{"style": "minimal"}},
			local:  map[string]any{"header": map[any]any{"hashes": "none"}},
			want:   `{"header":{"style":"minimal","hashes":"none"}}`,
		},
		{
			name:    "null local header does not wipe shared",
			shared:  map[string]any{"header": map[string]any{"style": "minimal"}},
			local:   map[string]any{"header": nil},
			wantErr: "key header has type <nil>, expected table",
		},
		{
			name:    "non-string name errors",
			shared:  map[string]any{},
			local:   map[string]any{"includes": []any{map[string]any{"name": 5}}},
			wantErr: "includes entry #1 (keys: name) has a name of type int",
		},
		{
			name:    "non-table shared entry errors without leaking value",
			shared:  map[string]any{"includes": []any{"s3cr3t-value"}},
			local:   map[string]any{"includes": []any{map[string]any{"name": "a"}}},
			wantErr: "includes entry #1 (type string) is not a table",
		},
		{
			name:    "non-table local entry errors",
			shared:  map[string]any{},
			local:   map[string]any{"plugins": []any{"s3cr3t-value"}},
			wantErr: "plugins entry #1 (type string) is not a table",
		},
		{
			name:    "shared list of wrong type errors",
			shared:  map[string]any{"plugins": map[string]any{"a": 1}},
			local:   map[string]any{"plugins": []any{}},
			wantErr: "shared config key plugins has type map[string]interface {}, expected list",
		},
		{
			name:   "empty merged list not created",
			shared: map[string]any{"name": "x"},
			local:  map[string]any{"plugins": []any{}},
			want:   `{"name":"x"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, _, err := MergeConfigDocs(tt.shared, tt.local)

			// Assert
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "s3cr3t") {
					t.Errorf("error leaks an entry value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			if !jsonEqual(t, string(gotJSON), tt.want) {
				t.Errorf("merged = %s, want %s", gotJSON, tt.want)
			}
		})
	}
}

func TestMergeConfigDocs_ErrorsNeverContainValues(t *testing.T) {
	// Arrange
	local := decodeDoc(t, "[[mcp_servers]]\ncommand = \"c\"\n[mcp_servers.env]\nTOKEN = \"s3cr3t\"")

	// Act
	_, _, err := MergeConfigDocs(map[string]any{}, local)

	// Assert
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error leaks a value: %v", err)
	}
}

func TestDocForJSON_RoundTripsSchemaIntoConfig(t *testing.T) {
	// Arrange
	shared := decodeDoc(t, "schema = \"https://example.com/a.json\"\nname = \"p\"")
	local := decodeDoc(t, `name = "q"`)

	// Act
	merged, _, err := MergeConfigDocs(shared, local)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := json.Marshal(DocForJSON(merged))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Assert
	if cfg.Schema != "https://example.com/a.json" || cfg.Name != "q" {
		t.Errorf("cfg = schema %q name %q", cfg.Schema, cfg.Name)
	}
	if _, ok := merged["schema"]; !ok {
		t.Error("DocForJSON must not mutate the merged document")
	}
}
