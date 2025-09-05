package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"gopkg.in/yaml.v3"
)

const (
	defaultPriority = "medium"
)

var (
	nonAlphanumericRE    = regexp.MustCompile(`[^a-z0-9]+`)
	consecutiveHyphensRE = regexp.MustCompile(`-+`)
)

func MigrateV1ToV2(configPath string) error {
	_, err := DetectAndMigrate(configPath)
	return err
}

func performMigrationSteps(config map[string]interface{}) ([]byte, error) {
	if err := migrateConfig(config); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	output, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal migrated config: %w", err)
	}

	return output, nil
}

func cleanupBackupFile(backupPath string) {
	log := logger.Get()
	if err := os.Remove(backupPath); err != nil {
		log.Warn("Failed to cleanup backup file after successful migration", "path", backupPath, "error", err)
	} else {
		log.Debug("Cleaned up backup file after successful migration", "path", backupPath)
	}
}

func detectV1Schema(config map[string]interface{}) (bool, error) {
	if schema, ok := config["$schema"].(string); ok {
		if strings.Contains(schema, "ai-rules-v1.schema.json") {
			return true, nil
		}
	}

	if outputs, ok := config["outputs"].([]interface{}); ok {
		for _, output := range outputs {
			if outputMap, ok := output.(map[string]interface{}); ok {
				if _, hasFile := outputMap["file"]; hasFile {
					return true, nil
				}

				if template, exists := outputMap["template"]; exists {
					if isV1Template(template) {
						return true, nil
					}
				}
			}
		}
	}

	if agents, ok := config["agents"].([]interface{}); ok {
		for _, agent := range agents {
			if agentMap, ok := agent.(map[string]interface{}); ok {
				if template, exists := agentMap["template"]; exists {
					if isV1Template(template) {
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

func isV1Template(template interface{}) bool {
	switch template.(type) {
	case string:
		return true
	case map[string]interface{}:
		return false
	default:
		return false
	}
}

func checkUnsupportedFeatures(config map[string]interface{}) error {
	if includes, exists := config["includes"]; exists {
		if includesList, ok := includes.([]interface{}); ok && len(includesList) > 0 {
			return fmt.Errorf(
				"cannot automatically migrate configuration with 'includes' field\n" +
					"Migration with includes is not supported because included files may also need migration.\n" +
					"Please either:\n" +
					"  1. Remove the includes and merge the configuration manually\n" +
					"  2. Migrate each included file separately first\n" +
					"  3. Convert to using 'extends' instead of 'includes'",
			)
		}
	}

	return nil
}

func migrateConfig(config map[string]interface{}) error {
	if _, hasUserRulez := config["user_rulez"]; hasUserRulez {
		return fmt.Errorf("user_rulez is no longer supported in v2 - use .local configuration files instead")
	}

	if _, ok := config["$schema"].(string); ok {
		config["$schema"] = "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json"
	}

	migratePrioritiesRelative(config)

	namedTargets := extractNamedTargets(config)

	migrationSteps := []struct {
		entityType    string
		migrationFunc func(map[string]interface{}) error
		needsTemplate bool
	}{
		{"outputs", migrateOutput, false},
		{"agents", migrateAgent, true},
		{"rules", migrateRule, true},
		{"sections", migrateSection, false},
	}

	for _, step := range migrationSteps {
		if entities, ok := config[step.entityType].([]interface{}); ok {
			for i, entity := range entities {
				if entityMap, ok := entity.(map[string]interface{}); ok {
					if err := step.migrationFunc(entityMap); err != nil {
						return fmt.Errorf("failed to migrate %s[%d]: %w", step.entityType, i, err)
					}
					if step.needsTemplate {
						if err := migrateTemplate(entityMap); err != nil {
							return fmt.Errorf("failed to migrate %s[%d]: %w", step.entityType, i, err)
						}
					}
					resolveTargetsInObject(entityMap, namedTargets)
				}
			}
		}
	}

	delete(config, "targets")

	return nil
}

func migrateOutput(output map[string]interface{}) error {
	if file, hasFile := output["file"]; hasFile {
		output["path"] = file
		delete(output, "file")
	}

	return migrateTemplate(output)
}

func migrateTemplate(obj map[string]interface{}) error {
	template, exists := obj["template"]
	if !exists {
		return nil
	}

	if _, isMap := template.(map[string]interface{}); isMap {
		return nil
	}

	if templateStr, ok := template.(string); ok {
		newTemplate := convertStringToTemplateObject(templateStr)
		obj["template"] = newTemplate
	}

	return nil
}

func convertStringToTemplateObject(templateStr string) map[string]interface{} {
	template := make(map[string]interface{})

	switch {
	case strings.HasPrefix(templateStr, "@"):
		template["type"] = "file"
		template["value"] = strings.TrimPrefix(templateStr, "@")
	case isBuiltinTemplate(templateStr):
		template["type"] = "builtin"
		template["value"] = templateStr
	default:
		template["type"] = "inline"
		template["value"] = templateStr
	}

	return template
}

func migrateAgent(agent map[string]interface{}) error {
	if name, hasName := agent["name"].(string); hasName {
		if _, hasID := agent["id"]; !hasID && name != "" {
			agent["id"] = generateHumanReadableID(name)
		}

		if _, hasDesc := agent["description"]; !hasDesc {
			agent["description"] = fmt.Sprintf("Agent: %s", name)
		}
	}

	return nil
}

func migrateSection(section map[string]interface{}) error {
	if title, hasTitle := section["title"]; hasTitle {
		section["name"] = title
		delete(section, "title")
	}

	if name, hasName := section["name"].(string); hasName {
		if _, hasID := section["id"]; !hasID && name != "" {
			section["id"] = generateHumanReadableID(name)
		}
	}

	return nil
}

func migrateRule(rule map[string]interface{}) error {
	return nil
}

func generateHumanReadableID(name string) string {
	if name == "" {
		return ""
	}

	id := strings.ToLower(name)
	id = nonAlphanumericRE.ReplaceAllString(id, "-")

	id = strings.Trim(id, "-")
	id = consecutiveHyphensRE.ReplaceAllString(id, "-")

	if len(id) > 50 {
		id = id[:50]
		id = strings.TrimRight(id, "-")
	}

	return id
}

func migratePriorityValue(priority interface{}, mapping map[int]string) string {
	intPriority := extractIntPriority(priority)
	if intPriority == 0 {
		return defaultPriority
	}

	if mapping != nil {
		if mapped, exists := mapping[intPriority]; exists {
			return mapped
		}
	}

	return defaultPriority
}

func migratePrioritiesRelative(config map[string]interface{}) {
	priorities := collectAllPriorities(config)

	if len(priorities) == 0 {
		return
	}

	priorityMap := createRelativePriorityMapping(priorities)

	migrateEntitiesPriorities(config, priorityMap)
}

func migrateEntitiesPriorities(config map[string]interface{}, mapping map[int]string) {
	entityTypes := []string{"rules", "agents", "sections"}
	for _, entityType := range entityTypes {
		if entities, ok := config[entityType].([]interface{}); ok {
			for _, entity := range entities {
				if entityMap, ok := entity.(map[string]interface{}); ok {
					migratePriorityInEntity(entityMap, mapping)
				}
			}
		}
	}
}

func migratePriorityInEntity(entity map[string]interface{}, mapping map[int]string) {
	if priority, hasPriority := entity["priority"]; hasPriority {
		entity["priority"] = migratePriorityValue(priority, mapping)
	}
}

func collectAllPriorities(config map[string]interface{}) []int {
	var priorities []int

	entityTypes := []string{"rules", "agents", "sections"}
	for _, entityType := range entityTypes {
		if entities, ok := config[entityType].([]interface{}); ok {
			for _, entity := range entities {
				if entityMap, ok := entity.(map[string]interface{}); ok {
					if priority := extractIntPriority(entityMap["priority"]); priority != 0 {
						priorities = append(priorities, priority)
					}
				}
			}
		}
	}

	return priorities
}

func extractIntPriority(priority interface{}) int {
	switch p := priority.(type) {
	case int:
		return p
	case float64:
		return int(p)
	case string:
		return parseIntPriority(p)
	default:
		return 0
	}
}

func createRelativePriorityMapping(priorities []int) map[int]string {
	if len(priorities) == 0 {
		return make(map[int]string)
	}

	uniquePriorities := make(map[int]bool)
	for _, p := range priorities {
		uniquePriorities[p] = true
	}

	sortedPriorities := make([]int, 0, len(uniquePriorities))
	for p := range uniquePriorities {
		sortedPriorities = append(sortedPriorities, p)
	}
	sort.Ints(sortedPriorities)

	mapping := make(map[int]string)
	n := len(sortedPriorities)

	if n == 1 {
		mapping[sortedPriorities[0]] = defaultPriority
		return mapping
	}

	enums := []string{"minimal", "low", "medium", "high", "critical"}

	for i, priority := range sortedPriorities {
		bucketIndex := (i * 5) / n
		if bucketIndex >= 5 {
			bucketIndex = 4
		}
		mapping[priority] = enums[bucketIndex]
	}

	return mapping
}

func parseIntPriority(s string) int {
	var val int
	if n, err := fmt.Sscanf(s, "%d", &val); err == nil && n == 1 {
		return val
	}
	return 0
}

func isBuiltinTemplate(name string) bool {
	builtinTemplates := []string{
		"default",
		"documentation",
		"minimal",
		"comprehensive",
		"agent",
		"specialized",
		"cursor",
		"windsurf",
		"aider",
		"continue",
	}

	for _, builtin := range builtinTemplates {
		if name == builtin {
			return true
		}
	}

	if !strings.Contains(name, " ") && !strings.Contains(name, "\n") &&
		!strings.Contains(name, "{") && !strings.Contains(name, "}") &&
		len(name) < 30 {
		return true
	}

	return false
}

func MigrateIfNeeded(configPath string) error {
	_, err := DetectAndMigrate(configPath)
	return err
}

func IsConfigFile(fileName string) bool {
	patterns := []string{
		"ai-rulez.yaml", "ai-rulez.yml",
		"ai-rules.yaml", "ai-rules.yml",
		"ai_rules.yaml", "ai_rulez.yaml",
		".ai-rulez.yaml", ".ai-rulez.yml",
		".ai-rules.yaml", ".ai-rules.yml",
	}

	lowerName := strings.ToLower(fileName)
	for _, pattern := range patterns {
		if lowerName == pattern {
			return true
		}
	}

	return false
}

func parseConfigFile(configPath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return config, nil
}

func performV1MigrationInline(config map[string]interface{}, configPath string) error {
	log := logger.Get()

	if err := checkUnsupportedFeatures(config); err != nil {
		return err
	}

	log.Info("Detected v1 configuration format, migrating to v2...")

	backupPath, err := BackupConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	log.Info("Created backup", "path", backupPath)

	originalInfo, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	output, err := performMigrationSteps(config)
	if err != nil {
		return err
	}

	tempPath, err := writeToTempFile(output, configPath, originalInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	if err := ValidateV2Schema(config); err != nil {
		if cleanupErr := os.Remove(tempPath); cleanupErr != nil {
			log.Warn("Failed to cleanup temp file after validation failure", "path", tempPath, "error", cleanupErr)
		}
		return handleV1MigrationError(backupPath, originalInfo.Mode(), configPath, err)
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		if cleanupErr := os.Remove(tempPath); cleanupErr != nil {
			log.Warn("Failed to cleanup temp file after rename failure", "path", tempPath, "error", cleanupErr)
		}
		return fmt.Errorf("failed to atomically replace config file: %w", err)
	}

	log.Info("Successfully migrated configuration from v1 to v2 format")
	cleanupBackupFile(backupPath)

	return nil
}

func handleV1MigrationError(backupPath string, originalMode os.FileMode, configPath string, validationErr error) error {
	log := logger.Get()
	log.Warn("Migration validation failed, restoring from backup and cleaning up", "error", validationErr)

	if backupData, readErr := os.ReadFile(backupPath); readErr == nil {
		if restoreErr := os.WriteFile(configPath, backupData, originalMode); restoreErr != nil {
			return fmt.Errorf("migration validation failed and backup restore failed: validation error: %w, restore error: %w", validationErr, restoreErr)
		}

		if cleanupErr := os.Remove(backupPath); cleanupErr != nil {
			log.Warn("Failed to cleanup backup file after restore", "path", backupPath, "error", cleanupErr)
		}

		return fmt.Errorf("migration validation failed, restored from backup: %w", validationErr)
	}

	if cleanupErr := os.Remove(backupPath); cleanupErr != nil {
		log.Warn("Failed to cleanup backup file", "path", backupPath, "error", cleanupErr)
	}

	return fmt.Errorf("migration validation failed and backup restore failed: %w", validationErr)
}

func DetectAndMigrate(configPath string) (string, error) {
	config, err := parseConfigFile(configPath)
	if err != nil {
		return "", err
	}

	needsV1Migration, err := detectV1Schema(config)
	if err != nil {
		return "unknown", err
	}

	var version string
	if needsV1Migration {
		version = "v1"
		if err := performV1MigrationInline(config, configPath); err != nil {
			return version, err
		}
	} else {
		version = "v2"
	}

	return version, nil
}

func PreviewMigration(configPath string) (*MigrationPreview, error) {
	config, err := parseConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	needsV1Migration, err := detectV1Schema(config)
	if err != nil {
		return nil, fmt.Errorf("failed to detect schema version: %w", err)
	}

	preview := &MigrationPreview{
		ConfigPath:     configPath,
		CurrentVersion: "v2",
		NeedsMigration: false,
		Changes:        []string{},
	}

	if !needsV1Migration {
		preview.Summary = "Configuration is already in v2 format - no migration needed"
		return preview, nil
	}

	preview.CurrentVersion = "v1"
	preview.NeedsMigration = true

	previewConfig := make(map[string]interface{})
	for k, v := range config {
		previewConfig[k] = v
	}

	if err := analyzeMigrationChanges(previewConfig, preview); err != nil {
		return nil, fmt.Errorf("failed to analyze migration changes: %w", err)
	}

	return preview, nil
}

type MigrationPreview struct {
	ConfigPath     string   `json:"config_path"`
	CurrentVersion string   `json:"current_version"`
	NeedsMigration bool     `json:"needs_migration"`
	Changes        []string `json:"changes"`
	Summary        string   `json:"summary"`
}

func analyzeMigrationChanges(config map[string]interface{}, preview *MigrationPreview) error { //nolint:gocyclo // Migration analysis requires checking many different configuration elements
	if schema, ok := config["$schema"].(string); ok && strings.Contains(schema, "v1") {
		preview.Changes = append(preview.Changes, "Update $schema URL from v1 to v2")
	}

	if outputs, ok := config["outputs"].([]interface{}); ok {
		fileCount := 0
		for _, output := range outputs {
			if outputMap, ok := output.(map[string]interface{}); ok {
				if _, hasFile := outputMap["file"]; hasFile {
					fileCount++
				}
			}
		}
		if fileCount > 0 {
			preview.Changes = append(preview.Changes, fmt.Sprintf("Migrate %d output(s): rename 'file' field to 'path'", fileCount))
		}
	}

	templateChanges := 0

	if agents, ok := config["agents"].([]interface{}); ok {
		for _, agent := range agents {
			if agentMap, ok := agent.(map[string]interface{}); ok {
				if template, exists := agentMap["template"]; exists {
					if _, isString := template.(string); isString {
						templateChanges++
					}
				}
			}
		}
	}

	if rules, ok := config["rules"].([]interface{}); ok {
		for _, rule := range rules {
			if ruleMap, ok := rule.(map[string]interface{}); ok {
				if template, exists := ruleMap["template"]; exists {
					if _, isString := template.(string); isString {
						templateChanges++
					}
				}
			}
		}
	}

	titleMigrations := 0
	if sections, ok := config["sections"].([]interface{}); ok {
		for _, section := range sections {
			if sectionMap, ok := section.(map[string]interface{}); ok {
				if _, hasTitle := sectionMap["title"]; hasTitle {
					titleMigrations++
				}
				if template, exists := sectionMap["template"]; exists {
					if _, isString := template.(string); isString {
						templateChanges++
					}
				}
			}
		}
	}

	if templateChanges > 0 {
		preview.Changes = append(preview.Changes, fmt.Sprintf("Convert %d template(s) from string to object format", templateChanges))
	}

	if titleMigrations > 0 {
		preview.Changes = append(preview.Changes, fmt.Sprintf("Migrate %d section(s): rename 'title' field to 'name'", titleMigrations))
	}

	idGenerations := 0

	if agents, ok := config["agents"].([]interface{}); ok {
		for _, agent := range agents {
			if agentMap, ok := agent.(map[string]interface{}); ok {
				if _, hasID := agentMap["id"]; !hasID {
					if name, hasName := agentMap["name"].(string); hasName && name != "" {
						idGenerations++
					}
				}
			}
		}
	}

	if sections, ok := config["sections"].([]interface{}); ok {
		for _, section := range sections {
			if sectionMap, ok := section.(map[string]interface{}); ok {
				if _, hasID := sectionMap["id"]; !hasID {
					var name string
					if n, hasName := sectionMap["name"].(string); hasName {
						name = n
					} else if t, hasTitle := sectionMap["title"].(string); hasTitle {
						name = t
					}
					if name != "" {
						idGenerations++
					}
				}
			}
		}
	}

	if idGenerations > 0 {
		preview.Changes = append(preview.Changes, fmt.Sprintf("Generate %d human-readable ID(s) for agents and sections", idGenerations))
	}

	namedTargets := extractNamedTargets(config)
	if len(namedTargets) > 0 {
		preview.Changes = append(preview.Changes, fmt.Sprintf("Resolve %d named target(s) and remove root-level targets section", len(namedTargets)))
	}

	if len(preview.Changes) == 0 {
		preview.Summary = "Configuration is already in v2 format - no migration needed"
	} else {
		preview.Summary = fmt.Sprintf("Will migrate from v1 to v2 format with %d change(s)", len(preview.Changes))
	}

	return nil
}

func getBackupPath(configPath string) string {
	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, fmt.Sprintf("%s.v1-backup%s", nameWithoutExt, ext))
}

func writeToTempFile(data []byte, targetPath string, mode os.FileMode) (string, error) {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	tempPath := filepath.Join(dir, fmt.Sprintf("%s.tmp-%d%s", nameWithoutExt, time.Now().UnixNano(), ext))

	if err := os.WriteFile(tempPath, data, mode); err != nil {
		return "", fmt.Errorf("failed to write temporary file: %w", err)
	}

	return tempPath, nil
}

func ValidateV2Schema(config map[string]interface{}) error {
	if outputs, ok := config["outputs"].([]interface{}); ok {
		for i, output := range outputs {
			if outputMap, ok := output.(map[string]interface{}); ok {
				if template, exists := outputMap["template"]; exists {
					if err := validateV2Template(template); err != nil {
						return fmt.Errorf("invalid template in output[%d]: %w", i, err)
					}
				}
			}
		}
	}

	if agents, ok := config["agents"].([]interface{}); ok {
		for i, agent := range agents {
			if agentMap, ok := agent.(map[string]interface{}); ok {
				if template, exists := agentMap["template"]; exists {
					if err := validateV2Template(template); err != nil {
						return fmt.Errorf("invalid template in agent[%d]: %w", i, err)
					}
				}
			}
		}
	}

	return nil
}

func validateV2Template(template interface{}) error {
	templateMap, ok := template.(map[string]interface{})
	if !ok {
		return fmt.Errorf("template must be an object with 'type' and 'value' fields, got %T", template)
	}

	templateType, hasType := templateMap["type"].(string)
	_, hasValue := templateMap["value"].(string)

	if !hasType || !hasValue {
		return fmt.Errorf("template must have both 'type' and 'value' fields")
	}

	validTypes := []string{"builtin", "file", "inline"}
	valid := false
	for _, vt := range validTypes {
		if templateType == vt {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("template type must be one of: builtin, file, inline")
	}

	return nil
}

func extractNamedTargets(config map[string]interface{}) map[string][]string {
	namedTargets := make(map[string][]string)

	if targets, ok := config["targets"].(map[string]interface{}); ok {
		for name, targetList := range targets {
			if targetSlice, ok := targetList.([]interface{}); ok {
				var stringTargets []string
				for _, target := range targetSlice {
					if targetStr, ok := target.(string); ok {
						stringTargets = append(stringTargets, targetStr)
					}
				}
				namedTargets[name] = stringTargets
			}
		}
	}

	return namedTargets
}

func resolveTargetsInObject(obj map[string]interface{}, namedTargets map[string][]string) {
	if targets, exists := obj["targets"]; exists {
		if targetList, ok := targets.([]interface{}); ok {
			var resolvedTargets []string

			for _, target := range targetList {
				if targetStr, ok := target.(string); ok {
					if strings.HasPrefix(targetStr, "@") {
						targetName := strings.TrimPrefix(targetStr, "@")
						if resolvedPatterns, found := namedTargets[targetName]; found {
							resolvedTargets = append(resolvedTargets, resolvedPatterns...)
						} else {
							resolvedTargets = append(resolvedTargets, targetStr)
						}
					} else {
						resolvedTargets = append(resolvedTargets, targetStr)
					}
				}
			}

			var interfaceTargets []interface{}
			for _, target := range resolvedTargets {
				interfaceTargets = append(interfaceTargets, target)
			}
			obj["targets"] = interfaceTargets
		}
	}
}

func BackupConfig(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)
	backupPath := filepath.Join(dir, fmt.Sprintf("%s.v1-backup%s", nameWithoutExt, ext))

	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}
