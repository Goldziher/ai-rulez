package fixtures

const BasicConfig = `metadata:
  name: "Test Project"
  version: "1.0.0"
  description: "A test project for ai-rulez"

presets:
  - claude

outputs:
  - path: "CLAUDE.md"
    template:
      type: builtin
      value: default

rules:
  - name: "Basic Rule"
    priority: medium
    content: "This is a basic rule for testing"

  - name: "High Priority Rule"
    priority: critical
    content: "This is a high priority rule"

sections:
  - name: "Development Guidelines"
    priority: high
    content: "Follow these guidelines for development"
`

const MinimalConfig = `metadata:
  name: "Minimal Project"

presets:
  - claude

outputs:
  - path: "output.md"
    template:
      type: builtin
      value: default

rules:
  - name: "Only Rule"
    content: "Single rule content"
`

const EmptyConfig = `metadata:
  name: "Empty Project"

presets:
  - claude

outputs:
  - path: "output.md"
    template:
      type: builtin
      value: default
`

const ConfigWithAllPriorities = `metadata:
  name: "Priority Test"

presets:
  - claude

outputs:
  - path: "output.md"
    template:
      type: builtin
      value: default

rules:
  - name: "Critical Rule"
    priority: critical
    content: "Critical priority"
    
  - name: "High Rule"  
    priority: high
    content: "High priority"
    
  - name: "Medium Rule"
    priority: medium
    content: "Medium priority"
    
  - name: "Low Rule"
    priority: low
    content: "Low priority"
    
sections:
  - name: "Critical Section"
    priority: critical
    content: "Critical section"
    
agents:
  - name: "critical-agent"
    description: "Critical agent"
    priority: critical
`

const ConfigWithTargets = `metadata:
  name: "Project with Targets"

presets:
  - claude

outputs:
  - path: "frontend.md"
    template:
      type: builtin
      value: default
  - path: "backend.md"
    template:
      type: builtin
      value: default

rules:
  - name: "Frontend Rule"
    content: "Frontend specific rule"
    targets: ["frontend.md", "src/**/*.jsx", "src/**/*.tsx"]

  - name: "Backend Rule"
    content: "Backend specific rule"
    targets: ["backend.md", "api/**/*.go", "internal/**/*.go"]

  - name: "Universal Rule"
    content: "Rule for everyone"
    
  - name: "Doc Rule"
    content: "Documentation rule"
    targets: ["docs/**/*.md", "*.md"]
`

const ConfigWithTemplate = `metadata:
  name: "Template Test"

presets:
  - claude

outputs:
  - path: "builtin.md"
    template:
      type: builtin
      value: minimal
      
  - path: "inline.md"
    template:
      type: inline
      value: |
        # {{.ProjectName}}
        {{range .Rules}}
        - {{.Name}}: {{.Content}}
        {{end}}
        
  - path: "file.md"
    template:
      type: file
      value: ./template.txt
      
rules:
  - name: "Template Rule"
    content: "Test template content"
`
