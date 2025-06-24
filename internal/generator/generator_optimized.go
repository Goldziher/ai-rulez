package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Goldziher/airules/internal/config"
	"github.com/Goldziher/airules/internal/templates"
)

// GenerateAllConcurrent generates all output files concurrently.
func (g *Generator) GenerateAllConcurrent(cfg *config.Config) error {
	if len(cfg.Outputs) == 0 {
		return errors.New("no outputs defined in configuration")
	}

	templateData := templates.NewTemplateData(cfg)

	// Use a wait group and error channel for concurrent processing
	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.Outputs))

	for i, output := range cfg.Outputs {
		wg.Add(1)
		go func(idx int, out config.Output) {
			defer wg.Done()
			if err := g.writeOutputFileOptimized(out, templateData); err != nil {
				errChan <- fmt.Errorf("failed to generate output %d (%s): %w", idx, out.File, err)
			}
		}(i, output)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return err // Return first error
	}

	return nil
}

// computeFileHash computes SHA256 hash of a file without loading entire content into memory.
func computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// shouldWriteFileOptimized checks if file should be written using streaming hash.
func (g *Generator) shouldWriteFileOptimized(filePath, newContent string) (bool, error) {
	fullPath := filepath.Join(g.baseDir, filePath)

	// If file doesn't exist, we should write it
	stat, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat file %s: %w", fullPath, err)
	}

	// For small files, use the original method
	if stat.Size() < 1024*1024 { // 1MB
		existingContent, err := os.ReadFile(fullPath)
		if err != nil {
			return false, fmt.Errorf("failed to read existing file %s: %w", fullPath, err)
		}
		existingHash := computeContentHash(string(existingContent))
		newHash := computeContentHash(newContent)
		return existingHash != newHash, nil
	}

	// For larger files, use streaming hash
	existingHash, err := computeFileHash(fullPath)
	if err != nil {
		return false, fmt.Errorf("failed to compute hash for %s: %w", fullPath, err)
	}

	newHash := computeContentHash(newContent)
	return existingHash != newHash, nil
}

// Hash pool to reduce allocations
var hashPool = sync.Pool{
	New: func() any {
		return sha256.New()
	},
}

// ComputeContentHashPooled computes SHA256 hash using a pooled hasher.
func ComputeContentHashPooled(content string) string {
	h := hashPool.Get().(hash.Hash)
	defer func() {
		h.Reset()
		hashPool.Put(h)
	}()

	// Pre-allocate the result buffer
	result := make([]byte, 0, sha256.Size)
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(result))
}

// renderTemplateOptimized renders a template using pooled string builders.
func (g *Generator) renderTemplateOptimized(output config.Output, data *templates.TemplateData) (string, error) {
	templateName := "default"
	if output.Template != "" {
		templateName = output.Template
	}

	// Check if this is a file reference (starts with @)
	if strings.HasPrefix(templateName, "@") {
		templatePath := strings.TrimPrefix(templateName, "@")
		fullPath := filepath.Join(g.baseDir, templatePath)

		// Read the template file
		templateContent, err := os.ReadFile(fullPath)
		if err != nil {
			return "", fmt.Errorf("failed to read template file %s: %w", fullPath, err)
		}

		// Register and render the template
		templateID := fmt.Sprintf("file:%s", templatePath)
		if err := g.renderer.RegisterTemplate(templateID, string(templateContent)); err != nil {
			return "", fmt.Errorf("failed to register template from %s: %w", templatePath, err)
		}

		return g.renderer.Render(templateID, data)
	}

	// Check if this is an inline template
	if strings.Contains(templateName, "\n") || strings.Contains(templateName, "{{") {
		return templates.RenderString(templateName, data)
	}

	// Otherwise, treat as a named template
	content, err := g.renderer.Render(templateName, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", templateName, err)
	}

	return content, nil
}

// writeOutputFileOptimized writes a single output file with optimizations.
func (g *Generator) writeOutputFileOptimized(output config.Output, data *templates.TemplateData) error {
	// Render the template
	content, err := g.renderTemplateOptimized(output, data)
	if err != nil {
		return err
	}

	// Check if we need to write the file
	shouldWrite, err := g.shouldWriteFileOptimized(output.File, content)
	if err != nil {
		return fmt.Errorf("failed to check if file should be written: %w", err)
	}
	if !shouldWrite {
		return nil // File content is unchanged, skip writing
	}

	// Write the file
	return g.writeFile(output.File, content)
}
