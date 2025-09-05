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

1.  **Locate the configuration struct:** Find the Go struct that maps to the `ai-rulez.yml` file. This is likely located in a file such as `internal/config/types.go`.
2.  **Add the `Presets` field:** Add a new field to this struct to capture the `presets` data.

    ```go
    Presets []string `yaml:"presets,omitempty"`
    ```

### 3.2. Create a Preset Registry

1.  **Create a new file:** Create a file named `internal/config/presets.go`.
2.  **Define the registry:** In this new file, define a global map that translates preset names into their corresponding `Output` objects. This registry will be the single source of truth for what each preset means.

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

1.  **Locate the configuration loading function:** Find the primary function responsible for loading and processing the `ai-rulez.yml` file. This is likely a function like `LoadConfig` or `LoadConfigWithIncludes` in the `internal/config/` package.
2.  **Modify the function:** After the YAML file has been unmarshalled into the configuration struct, perform the following steps:
    *   Check if the `Presets` field in the config struct is populated.
    *   If it is, create a new list of `Output` objects.
    *   Iterate through each `presetName` in `config.Presets`.
    *   Look up `presetName` in the `PresetRegistry` and append the resulting `[]Output` to the new list.
    *   Merge the list of `Output` objects generated from `presets` with the existing `config.Outputs` list.
    *   **Implement de-duplication:** Ensure that the final list of outputs does not contain duplicates. An explicit `Output` defined in the `outputs` key should always take precedence over an `Output` generated from a preset. A map-based approach using the `Output.Path` as the key is recommended for handling this.

---

## 4. Phase 3: Testing

**Location for new tests:** `internal/config/`

Create a new test file (e.g., `presets_test.go`) to validate the new functionality. The tests should cover the following scenarios:

-   A configuration with only the `presets` key is loaded correctly.
-   A configuration with only the `outputs` key is loaded correctly.
-   A configuration with both `presets` and `outputs` keys merges the results correctly.
-   When a duplicate output path exists in both `presets` and `outputs`, the one from `outputs` is used (it should overwrite the preset-derived one).
-   Using an invalid preset name in the YAML file results in a validation error (this should be handled by the schema, but a unit test is good practice).

---

## 5. Phase 4: Documentation

Once the feature is implemented and tested, update the project documentation.

1.  **`README.md`:** Update the main example to use the `presets` key instead of `outputs` to showcase the new, simpler format.
2.  **`docs/configuration.md`:** Add a section explaining the `presets` key and how it works alongside `outputs`.
3.  **`docs/examples.md`:** Add a new example demonstrating a configuration that uses `presets`.
