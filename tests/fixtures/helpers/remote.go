package fixtures

const ConfigWithExtends = `metadata:
  name: "Child Project"
  version: "2.0.0"
  
extends: "./base.yaml"

outputs:
  - path: "child-output.md"
  
rules:
  - name: "Child Rule"
    content: "Additional rule from child"
`

const BaseConfig = `metadata:
  name: "Base Project"
  version: "1.0.0"
  description: "Base configuration"
  
outputs:
  - path: "base-output.md"
  
rules:
  - name: "Base Rule"
    content: "Rule from base config"
    priority: high
    
sections:
  - name: "Base Section"
    content: "Section from base config"
`

const ConfigWithIncludes = `metadata:
  name: "Main Project"
  
includes:
  - "./rules.yaml"
  - "./sections.yaml"
  - "./agents.yaml"
  
outputs:
  - path: "main-output.md"
  
rules:
  - name: "Main Rule"
    content: "Rule in main config"
`

const IncludedRulesConfig = `rules:
  - name: "Included Rule 1"
    content: "First included rule"
    priority: high
    
  - name: "Included Rule 2"
    content: "Second included rule"
    priority: low
`

const IncludedSectionsConfig = `sections:
  - name: "Included Section 1"
    content: "First included section"
    
  - name: "Included Section 2"
    content: "Second included section"
`

const IncludedAgentsConfig = `agents:
  - name: "included-agent"
    description: "Agent from included file"
    tools: ["Read", "Write"]
    system_prompt: "You are an included agent"
`

const ConfigWithRemoteIncludes = `metadata:
  name: "Remote Include Test"
  
includes:
  - "https://raw.githubusercontent.com/example/repo/main/rules.yaml"
  - "./local-rules.yaml"
  
outputs:
  - path: "output.md"
`

const ConfigWithMixedInheritance = `metadata:
  name: "Mixed Inheritance"
  version: "3.0.0"
  
extends: "./base.yaml"

includes:
  - "./extra-rules.yaml"
  - "./extra-sections.yaml"
  
outputs:
  - path: "mixed-output.md"
  
rules:
  - name: "Override Rule"
    content: "This overrides a base rule"
`

const ConfigWithNamedTargetsInheritance = `metadata:
  name: "Named Targets Test"
  
extends: "./base-with-targets.yaml"

named_targets:
  test-files: ["**/*_test.go", "**/*.test.ts"]
  
rules:
  - name: "Test Rule"
    content: "Rule for test files"
    targets: ["@test-files"]
`

const BaseConfigWithNamedTargets = `metadata:
  name: "Base with Targets"
  
outputs:
  - path: "output.md"
  
named_targets:
  go-files: ["**/*.go"]
  ts-files: ["**/*.ts", "**/*.tsx"]
  doc-files: ["**/*.md"]
  
rules:
  - name: "Go Rule"
    content: "Rule for Go files"
    targets: ["@go-files"]
`
