package hooks

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	lefthookSystem        = "lefthook"
	preCommitSystem       = "pre-commit"
	huskySystem           = "husky"
	officialPreCommitRepo = "https://github.com/Goldziher/ai-rulez"
	officialPreCommitRev  = "v4.12.0"
	keyRepo               = "repo"
	keyHooks              = "hooks"
	binaryAIRulez         = "ai-rulez"
	unknownLabel          = "Unknown"
	nullTag               = "!!null"
)

// yamlField is an ordered key/value pair. Emitting from a map would reorder the
// fields on every invocation, since Go randomizes map iteration.
type yamlField struct {
	Key   string
	Value string
}

func SetupHooks() error {
	hookSystem := DetectGitHooks()
	switch hookSystem {
	case lefthookSystem:
		return setupLefthook()
	case preCommitSystem:
		return setupPreCommit()
	case huskySystem:
		return setupHusky()
	default:
		return fmt.Errorf("no git hook system detected (lefthook, pre-commit, or husky)")
	}
}

// setupLefthook adds ai-rulez validation to lefthook configuration while preserving
// comments, formatting, and key order using yaml.Node for comment-preserving round-tripping.
func setupLefthook() error {
	configFile := ""
	files := []string{"lefthook.yaml", "lefthook.yml", ".lefthook.yml", ".lefthook.yaml"}
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			configFile = file
			break
		}
	}

	if configFile == "" {
		return fmt.Errorf("lefthook configuration file not found")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read lefthook config: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("failed to parse lefthook config: %w", err)
	}

	// The root is a Document node, we need the first child (the mapping)
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("invalid lefthook config: expected document node")
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return fmt.Errorf("invalid lefthook config: expected mapping at root")
	}

	// Find or create pre-commit section
	preCommitNode := findOrCreateMapKey(doc, "pre-commit", yaml.MappingNode)
	if !coerceNodeKind(preCommitNode, yaml.MappingNode) {
		return fmt.Errorf("invalid lefthook config: pre-commit must be a mapping")
	}

	// Find or create commands section under pre-commit
	commandsNode := findOrCreateMapKey(preCommitNode, "commands", yaml.MappingNode)
	if !coerceNodeKind(commandsNode, yaml.MappingNode) {
		return fmt.Errorf("invalid lefthook config: commands must be a mapping")
	}

	// Check if ai-rulez command already exists
	if hasMapKey(commandsNode, binaryAIRulez) {
		return nil
	}

	// Add ai-rulez command
	addMapEntry(commandsNode, binaryAIRulez, []yamlField{
		{Key: "glob", Value: ".ai-rulez/**"},
		{Key: "run", Value: "ai-rulez validate"},
		{Key: "fail_text", Value: "AI rules validation failed"},
	})

	updatedData, err := marshalYAMLPreservingIndent(&root, data)
	if err != nil {
		return fmt.Errorf("failed to marshal lefthook config: %w", err)
	}

	if err := os.WriteFile(configFile, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write lefthook config: %w", err)
	}

	return nil
}

// marshalYAMLPreservingIndent encodes a node tree using the indentation width of
// the document it came from.
//
// yaml.Marshal hardcodes a four-space indent, so round-tripping a two-space file
// re-indents every line of it. Comments survive that, but the diff still rewrites
// the whole file — on a long hand-maintained lefthook.yml that is just as
// unwelcome as losing the comments was (#186).
//
// One kind of formatting still cannot be preserved: yaml.v3 does not model blank
// lines between entries, so they are dropped, and padding used to align trailing
// comments collapses to a single space. Everything else — comments, key order,
// quoting style, indentation width — round-trips.
func marshalYAMLPreservingIndent(root *yaml.Node, original []byte) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(detectYAMLIndent(original))
	encodeErr := encoder.Encode(root)
	// Close flushes, so its error matters even when Encode succeeded — but an
	// Encode failure is the more specific diagnosis and wins.
	closeErr := encoder.Close()
	if encodeErr != nil {
		return nil, encodeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return buf.Bytes(), nil
}

// detectYAMLIndent infers a document's indentation width from the first line
// that is indented at all, ignoring comments and block scalar bodies by taking
// the smallest non-zero indent seen on a line that ends in a mapping key.
//
// Falls back to two spaces, which is the prevailing convention for the hook
// configs this package edits and matches what yaml.v3 would produce least
// disruptively.
func detectYAMLIndent(data []byte) int {
	const defaultIndent = 2
	smallest := 0
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		// Only mapping keys are reliable: a block scalar's body can be indented
		// arbitrarily deep and would understate nothing but confuse the minimum.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, ":") {
			continue
		}
		width := len(line) - len(trimmed)
		if width == 0 || strings.Contains(line, "\t") {
			continue
		}
		if smallest == 0 || width < smallest {
			smallest = width
		}
	}
	if smallest == 0 {
		return defaultIndent
	}
	return smallest
}

// findOrCreateMapKey searches for a key in a mapping node and returns its value node.
// If the key doesn't exist, it creates an empty value node of the requested kind.
// An existing value is returned as it stands, kind included — coerceNodeKind decides
// whether it is usable.
func findOrCreateMapKey(mapping *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}

	// Search for existing key
	for i := 0; i < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		if keyNode.Value == key {
			return mapping.Content[i+1]
		}
	}

	// Key not found, create it
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}
	valueNode := &yaml.Node{
		Kind: kind,
	}

	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

// hasMapKey checks if a mapping node contains a specific key.
func hasMapKey(mapping *yaml.Node, key string) bool {
	if mapping.Kind != yaml.MappingNode {
		return false
	}

	for i := 0; i < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		if keyNode.Value == key {
			return true
		}
	}
	return false
}

// coerceNodeKind reports whether node ends up with the wanted kind, treating a
// `!!null` scalar as an absent value and filling it in.
//
// A key written without a value (`commands:`, `repos:`) is legal YAML and a legal
// placeholder in both hook configs, so it deserves to be populated rather than
// rejected. A node holding any real value keeps it and is reported as a mismatch,
// so callers can refuse instead of discarding the user's data.
//
// The node is replaced wholesale rather than having its Kind reassigned: yaml.v3
// only elides a non-scalar node's tag when it matches the kind's implicit tag, so
// a leftover `!!null` Tag on a sequence or mapping is written out explicitly.
func coerceNodeKind(node *yaml.Node, kind yaml.Kind) bool {
	if node == nil {
		return false
	}
	if node.Kind == kind {
		return true
	}
	if node.Kind != yaml.ScalarNode || node.Tag != nullTag {
		return false
	}
	*node = yaml.Node{Kind: kind}
	return true
}

// addMapEntry adds a new key-value pair to a mapping node, whose value is a
// mapping built from fields in the order given.
func addMapEntry(mapping *yaml.Node, key string, fields []yamlField) {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	valueNode := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, field := range fields {
		fieldKey := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: field.Key,
		}
		fieldValue := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: field.Value,
		}
		valueNode.Content = append(valueNode.Content, fieldKey, fieldValue)
	}

	mapping.Content = append(mapping.Content, keyNode, valueNode)
}

func setupPreCommit() error {
	configFile, err := findPreCommitConfig()
	if err != nil {
		return err
	}

	root, original, err := readPreCommitConfig(configFile)
	if err != nil {
		return err
	}

	// Get the document's content (mapping node)
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("invalid pre-commit config: expected document node")
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return fmt.Errorf("invalid pre-commit config: expected mapping at root")
	}

	// Find or create repos array
	reposNode := findOrCreateMapKey(doc, "repos", yaml.SequenceNode)
	if !coerceNodeKind(reposNode, yaml.SequenceNode) {
		return fmt.Errorf("invalid pre-commit config: repos must be a sequence")
	}

	if err := ensureOfficialPreCommitRepoNode(reposNode); err != nil {
		return err
	}
	pruneLegacyLocalHooksNode(reposNode)

	return writePreCommitConfig(configFile, root, original)
}

func findPreCommitConfig() (string, error) {
	candidates := []string{".pre-commit-config.yaml", "pre-commit-config.yaml"}
	for _, file := range candidates {
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}
	}
	return "", fmt.Errorf("pre-commit configuration file not found")
}

// readPreCommitConfig reads the pre-commit config as a yaml.Node to preserve
// comments. The raw bytes are returned alongside it so the writer can reproduce
// the document's own indentation width.
func readPreCommitConfig(path string) (*yaml.Node, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read pre-commit config: %w", err)
	}

	if len(data) == 0 {
		// Return an empty document with a mapping node
		root := &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode},
			},
		}
		return root, data, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("failed to parse pre-commit config: %w", err)
	}
	return &root, data, nil
}

// ensureOfficialPreCommitRepoNode ensures the official ai-rulez repo exists in the repos sequence
// and has the correct hooks, while preserving comments and formatting.
func ensureOfficialPreCommitRepoNode(reposNode *yaml.Node) error {
	if reposNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("invalid pre-commit config: repos must be a sequence")
	}

	// Look for existing official repo
	var officialRepoNode *yaml.Node
	for _, repo := range reposNode.Content {
		if repo.Kind != yaml.MappingNode {
			continue
		}
		repoURL := getMapValue(repo, keyRepo)
		if repoURL == officialPreCommitRepo {
			officialRepoNode = repo
			// Update rev
			setMapValue(repo, "rev", officialPreCommitRev)
			break
		}
	}

	// If official repo not found, create it
	if officialRepoNode == nil {
		officialRepoNode = &yaml.Node{
			Kind: yaml.MappingNode,
		}
		setMapValue(officialRepoNode, keyRepo, officialPreCommitRepo)
		setMapValue(officialRepoNode, "rev", officialPreCommitRev)

		// Create hooks array
		hooksKey := &yaml.Node{Kind: yaml.ScalarNode, Value: keyHooks}
		hooksValue := &yaml.Node{Kind: yaml.SequenceNode}
		officialRepoNode.Content = append(officialRepoNode.Content, hooksKey, hooksValue)

		reposNode.Content = append(reposNode.Content, officialRepoNode)
	}

	// Ensure hooks exist
	return ensureOfficialHooksNode(officialRepoNode)
}

// ensureOfficialHooksNode ensures the official hooks are present in a repo node.
func ensureOfficialHooksNode(repoNode *yaml.Node) error {
	hooksNode := findOrCreateMapKey(repoNode, keyHooks, yaml.SequenceNode)
	if !coerceNodeKind(hooksNode, yaml.SequenceNode) {
		return fmt.Errorf("invalid pre-commit config: hooks must be a sequence")
	}

	// Build set of existing hook IDs
	existingIDs := make(map[string]bool)
	for _, hook := range hooksNode.Content {
		if hook.Kind != yaml.MappingNode {
			continue
		}
		id := getMapValue(hook, "id")
		if id != "" {
			existingIDs[id] = true
		}
	}

	// Add missing hooks
	for _, id := range []string{"ai-rulez-validate", "ai-rulez-generate"} {
		if existingIDs[id] {
			continue
		}
		hookNode := &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(hookNode, "id", id)
		hooksNode.Content = append(hooksNode.Content, hookNode)
	}

	return nil
}

// pruneLegacyLocalHooksNode removes legacy ai-rulez hooks from local repos.
func pruneLegacyLocalHooksNode(reposNode *yaml.Node) {
	if reposNode.Kind != yaml.SequenceNode {
		return
	}

	for _, repo := range reposNode.Content {
		if repo.Kind != yaml.MappingNode {
			continue
		}

		repoURL := getMapValue(repo, keyRepo)
		if repoURL != "local" {
			continue
		}

		// Find hooks
		var hooksNode *yaml.Node
		for i := 0; i < len(repo.Content); i += 2 {
			if repo.Content[i].Value == keyHooks {
				hooksNode = repo.Content[i+1]
				break
			}
		}

		if hooksNode == nil || hooksNode.Kind != yaml.SequenceNode {
			continue
		}

		// Filter out ai-rulez hooks
		filtered := []*yaml.Node{}
		for _, hook := range hooksNode.Content {
			if hook.Kind != yaml.MappingNode {
				filtered = append(filtered, hook)
				continue
			}
			id := getMapValue(hook, "id")
			if id == binaryAIRulez {
				continue
			}
			filtered = append(filtered, hook)
		}
		hooksNode.Content = filtered
	}
}

// getMapValue retrieves a string value from a mapping node by key.
func getMapValue(mapping *yaml.Node, key string) string {
	if mapping.Kind != yaml.MappingNode {
		return ""
	}

	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key && i+1 < len(mapping.Content) {
			return mapping.Content[i+1].Value
		}
	}
	return ""
}

// setMapValue sets or updates a string value in a mapping node.
//
// An existing value node is replaced rather than having its Value reassigned.
// yaml.v3 records the tag it inferred while parsing, and writes that tag out
// explicitly once it no longer matches the value: an unquoted `rev: 24` carries
// !!int, so assigning "v4.11.5" in place would emit `rev: !!int v4.11.5`, which
// pre-commit then rejects.
func setMapValue(mapping *yaml.Node, key, value string) {
	if mapping.Kind != yaml.MappingNode {
		return
	}

	// Look for existing key
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			existing := mapping.Content[i+1]
			*existing = yaml.Node{
				Kind:        yaml.ScalarNode,
				Value:       value,
				HeadComment: existing.HeadComment,
				LineComment: existing.LineComment,
				FootComment: existing.FootComment,
			}
			return
		}
	}

	// Key not found, add it
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valueNode := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
}

// writePreCommitConfig writes a yaml.Node back to file, preserving comments and
// the original indentation width. original is the file's contents as read, used
// only to detect that width.
func writePreCommitConfig(path string, root *yaml.Node, original []byte) error {
	data, err := marshalYAMLPreservingIndent(root, original)
	if err != nil {
		return fmt.Errorf("failed to marshal pre-commit config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write pre-commit config: %w", err)
	}

	return nil
}

func setupHusky() error {
	if _, err := os.Stat(".husky"); os.IsNotExist(err) {
		return fmt.Errorf(".husky directory not found")
	}

	var hookContent string

	if data, err := os.ReadFile(".husky/pre-commit"); err == nil {
		hookContent = string(data)

		if strings.Contains(hookContent, binaryAIRulez) {
			return nil
		}

		if !strings.HasSuffix(hookContent, "\n") {
			hookContent += "\n"
		}
		hookContent += "\n# Validate AI rules configuration\n"
		hookContent += "echo 'Validating AI rules...'\n"
		hookContent += "npx ai-rulez validate || exit 1\n"
	} else {
		hookContent = `#!/usr/bin/env sh
. "$(dirname -- "$0")/_/husky.sh"

# Validate AI rules configuration
echo 'Validating AI rules...'
npx ai-rulez validate || exit 1
`
	}

	huskyRoot, err := os.OpenRoot(".husky")
	if err != nil {
		return fmt.Errorf("failed to open .husky directory: %w", err)
	}
	defer func() { _ = huskyRoot.Close() }()

	if err := huskyRoot.WriteFile("pre-commit", []byte(hookContent), 0o755); err != nil {
		return fmt.Errorf("failed to write husky pre-commit hook: %w", err)
	}

	return nil
}

func GetHookSystemName(system string) string {
	switch system {
	case lefthookSystem:
		return "Lefthook"
	case preCommitSystem:
		return "Pre-commit"
	case huskySystem:
		return "Husky"
	default:
		return unknownLabel
	}
}
