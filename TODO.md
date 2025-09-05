# Implementation Plan: `presets` Feature

This document outlines the requirements and implementation steps for adding a top-level `presets` key to the `ai-rulez.yml` configuration file. This feature is intended to simplify the user experience by providing a shorthand for common output configurations.

## 1. Objective

Introduce a `presets` key in `ai-rulez.yml` as a simpler alternative to the `outputs` key. The final configuration must contain at least one of `presets` or `outputs`.

---

## 2. Phase 1: JSON Schema Modification

**File to Modify:** `schema/ai-rules-v2.schema.json`

This phase ensures that editors provide autocompletion and validation for the new feature.

### 2.1. Update Top-Level Requirements

1.  **Locate the top-level `required` array:** Find the line `"required": ["metadata", "outputs"]`.
2.  **Modify the array:** Remove `"outputs"` from this array. The line should now be `"required": ["metadata"]`.

### 2.2. Enforce `outputs` or `presets` Requirement

1.  **Add an `anyOf` block:** Directly under the top-level `required` array, add the following `anyOf` block. This makes it mandatory for a configuration to contain at least one of the two keys.

    ```json
    "anyOf": [
      { "required": ["outputs"] },
      { "required": ["presets"] }
    ],
    ```

### 2.3. Define the `presets` Property

1.  **Add the `presets` property:** Inside the top-level `properties` object (alongside `metadata`, `outputs`, etc.), add the following definition for the `presets` key.

    ```json
    "presets": {
      "type": "array",
      "description": "List of predefined output configurations to generate",
      "minItems": 1,
      "items": {
        "type": "string",
        "enum": ["popular", "claude", "cursor", "gemini", "copilot", "continue", "windsurf", "cline"],
        "description": "Name of the preset to use"
      },
      "uniqueItems": true
    },
    ```

---

## 3. Phase 2: Application Logic Implementation

This phase involves modifying the Go source code to handle the new `presets` key.

### 3.1. Update Configuration Struct

1.  **Locate the configuration struct:** Find the Go struct that maps to the `ai-rulez.yml` file (likely in `internal/config/types.go`).
2.  **Add the `Presets` field:** Add a new field to this struct to capture the `presets` data.

    ```go
    Presets []string `yaml:"presets,omitempty"`
    ```

### 3.2. Create a Preset Registry

1.  **Create a new file:** `internal/config/presets.go`.
2.  **Define the registry:** In this new file, define a global map that translates preset names into their corresponding `Output` objects.

    ```go
    // internal/config/presets.go
    package config

    // PresetRegistry maps preset names to their corresponding Output configurations.
    var PresetRegistry = map[string][]Output{
        "claude": {
            {Path: "CLAUDE.md"},
        },
        "cursor": {
            {Path: ".cursor/rules/", Type: "rule", NamingScheme: "{name}.mdc"},
        },
        "copilot": {
            {Path: ".github/copilot-instructions.md"},
        },
        // Add other supported presets like gemini, continue, etc.
        "popular": {
            {Path: "CLAUDE.md"},
            {Path: ".cursor/rules/", Type: "rule", NamingScheme: "{name}.mdc"},
            {Path: ".github/copilot-instructions.md"},
        },
    }
    ```

### 3.3. Implement Preset Expansion and Merging

1.  **Locate the configuration loading function:** Find the primary function responsible for loading and processing the `ai-rulez.yml` file (likely `LoadConfig` or `LoadConfigWithIncludes` in the `internal/config/` package).
2.  **Modify the function:** After unmarshalling the YAML, perform the following steps:
    *   If `config.Presets` is populated, iterate through each `presetName`, look it up in the `PresetRegistry`, and append the resulting `[]Output` to a new list.
    *   Merge this new list with the existing `config.Outputs` list.
    *   **Implement de-duplication:** Ensure the final list of outputs is unique. An explicit `Output` in the `outputs` key should always take precedence over one from a preset. A map-based approach using `Output.Path` as the key is recommended.

### 3.4. Update `init` Command Logic

1.  **Locate the `init` command file:** Find the source file for the `ai-rulez init` command.
2.  **Modify the logic:**
    *   If the `init` command is run with a `--preset` or `--popular` flag, the generated `ai-rulez.yml` file **must** use the `presets` key.
        *   **Example (`ai-rulez init --preset popular`):**
            ```yaml
            metadata:
              name: "My Project"
            presets: ["popular"]
            ```
    *   If the `init` command is run *without* a preset flag, the generated `ai-rulez.yml` file **must** use the `outputs` key with a default configuration.
        *   **Example (`ai-rulez init`):**
            ```yaml
            metadata:
              name: "My Project"
            outputs:
              - path: "CLAUDE.md"
            ```

---

## 4. Phase 3: Testing

**Location for new tests:** `internal/config/` and the `init` command's test file.

Create a new test file (e.g., `presets_test.go`) and update existing tests to cover the following scenarios:

-   A configuration with only `presets` is loaded correctly.
-   A configuration with only `outputs` is loaded correctly.
-   A configuration with both `presets` and `outputs` merges the results correctly, with `outputs` taking precedence on conflicts.
-   `ai-rulez init --preset popular` generates a config with the `presets` key.
-   `ai-rulez init` (no flags) generates a config with the `outputs` key.
-   Using an invalid preset name in the YAML file results in a validation error.

---

## 5. Phase 4: Documentation

Once the feature is implemented and tested, update the project documentation with the following changes:

1.  **`README.md`:**
    *   Update the main example to use `presets: ["popular"]` to showcase the new, simpler format.

2.  **`docs/quick-start.md`:**
    *   Ensure the `init` command example uses the `--popular` flag.
    *   Show the resulting `ai-rulez.yml` file using the `presets` key.

3.  **`docs/configuration.md`:**
    *   Add a new top-level section for the `presets` key.
    *   Explain that `presets` is a shorthand for common configurations.
    *   Explain that `outputs` provides more fine-grained control for custom setups.
    *   Document the merging behavior: when both keys are present, they are combined, and `outputs` entries will override any duplicates from `presets`.

4.  **`docs/examples.md`:**
    *   Add a new example showing a `presets`-only configuration.
    *   Add another example showing a mixed configuration that uses both `presets` and `outputs` to demonstrate the override/merge functionality.
