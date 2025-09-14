package enforcement

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

// Fixer applies fixes to files based on violation suggestions
type Fixer struct{}

// NewFixer creates a new file fixer
func NewFixer() *Fixer {
	return &Fixer{}
}

// ApplyFix applies a violation fix to the file
func (f *Fixer) ApplyFix(violation *RuleViolation) error {
	if violation.Fix == nil {
		return fmt.Errorf("no fix provided for violation")
	}

	fix := violation.Fix
	filePath := violation.FilePath

	logger.Debug("Applying fix",
		"file", filePath,
		"rule", violation.RuleName,
		"description", fix.Description)

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	lines := strings.Split(string(content), "\n")

	// Apply line-based fix
	if fix.StartLine > 0 && fix.EndLine > 0 {
		return f.applyLineFix(filePath, lines, fix)
	}

	// Apply text-based fix
	if fix.OldText != "" && fix.NewText != "" {
		return f.applyTextFix(filePath, string(content), fix.OldText, fix.NewText)
	}

	return fmt.Errorf("fix does not specify how to apply changes")
}

func (f *Fixer) applyLineFix(filePath string, lines []string, fix *ViolationFix) error {
	if fix.StartLine < 1 || fix.EndLine < 1 || fix.StartLine > len(lines) || fix.EndLine > len(lines) {
		return fmt.Errorf("invalid line range: %d-%d (file has %d lines)", fix.StartLine, fix.EndLine, len(lines))
	}

	// Convert to 0-based indexing
	startIdx := fix.StartLine - 1
	endIdx := fix.EndLine - 1

	// Replace lines
	var newLines []string

	// Add lines before the change
	newLines = append(newLines, lines[:startIdx]...)

	// Add the new text (split into lines if it contains newlines)
	if fix.NewText != "" {
		newTextLines := strings.Split(fix.NewText, "\n")
		newLines = append(newLines, newTextLines...)
	}

	// Add lines after the change
	if endIdx+1 < len(lines) {
		newLines = append(newLines, lines[endIdx+1:]...)
	}

	// Write back to file
	newContent := strings.Join(newLines, "\n")
	return f.writeFile(filePath, newContent)
}

func (f *Fixer) applyTextFix(filePath, content, oldText, newText string) error {
	// Simple text replacement
	if !strings.Contains(content, oldText) {
		return fmt.Errorf("old text not found in file: %s", oldText)
	}

	// Replace first occurrence only to be safe
	newContent := strings.Replace(content, oldText, newText, 1)

	return f.writeFile(filePath, newContent)
}

func (f *Fixer) writeFile(filePath, content string) error {
	// Create backup
	backupPath := filePath + ".bak"
	if err := f.createBackup(filePath, backupPath); err != nil {
		logger.Warn("Failed to create backup", "file", filePath, "error", err)
	}

	// Write new content
	err := os.WriteFile(filePath, []byte(content), 0o644)
	if err != nil {
		// Try to restore from backup
		if restoreErr := f.restoreBackup(backupPath, filePath); restoreErr != nil {
			logger.Error("Failed to restore backup after write failure", "file", filePath, "error", restoreErr)
		}
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	// Remove backup on success
	if err := os.Remove(backupPath); err != nil {
		logger.Debug("Failed to remove backup", "backup", backupPath, "error", err)
	}

	return nil
}

func (f *Fixer) createBackup(originalPath, backupPath string) error {
	original, err := os.Open(originalPath)
	if err != nil {
		return err
	}
	defer original.Close()

	backup, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer backup.Close()

	scanner := bufio.NewScanner(original)
	writer := bufio.NewWriter(backup)
	defer writer.Flush()

	for scanner.Scan() {
		if _, err := writer.WriteString(scanner.Text() + "\n"); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func (f *Fixer) restoreBackup(backupPath, originalPath string) error {
	backup, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer backup.Close()

	original, err := os.Create(originalPath)
	if err != nil {
		return err
	}
	defer original.Close()

	scanner := bufio.NewScanner(backup)
	writer := bufio.NewWriter(original)
	defer writer.Flush()

	for scanner.Scan() {
		if _, err := writer.WriteString(scanner.Text() + "\n"); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// ApplyAllFixes applies fixes for multiple violations
func (f *Fixer) ApplyAllFixes(violations []RuleViolation) (int, []error) {
	var errors []error
	applied := 0

	// Group violations by file to batch fixes
	fileViolations := make(map[string][]*RuleViolation)
	for i := range violations {
		if violations[i].Fix != nil {
			fileViolations[violations[i].FilePath] = append(fileViolations[violations[i].FilePath], &violations[i])
		}
	}

	// Apply fixes file by file
	for filePath, fileViolationList := range fileViolations {
		logger.Info("Applying fixes to file", "file", filePath, "count", len(fileViolationList))

		for _, violation := range fileViolationList {
			if err := f.ApplyFix(violation); err != nil {
				logger.Warn("Failed to apply fix", "file", filePath, "rule", violation.RuleName, "error", err)
				errors = append(errors, fmt.Errorf("fix failed for %s in %s: %w", violation.RuleName, filePath, err))
				continue
			}
			applied++
		}
	}

	return applied, errors
}
