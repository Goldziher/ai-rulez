package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// TestNewWithRenderer tests the NewWithRenderer constructor
func TestNewWithRenderer(t *testing.T) {
	t.Parallel()

	renderer := templates.NewRenderer()
	gen := NewWithRenderer(renderer)

	assert.NotNil(t, gen)
	assert.Equal(t, "", gen.configFile)
	assert.Equal(t, ".", gen.baseDir)
	assert.Equal(t, renderer, gen.renderer)
}

// TestGenerateAllConcurrent tests concurrent generation
func TestGenerateAllConcurrent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a config with many outputs to trigger concurrent processing
	outputs := make([]config.Output, 15)
	for i := 0; i < 15; i++ {
		outputs[i] = config.Output{
			Path: fmt.Sprintf("output%d.md", i),
		}
	}

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:    "Concurrent Test",
			Version: "1.0.0",
		},
		Outputs: outputs,
		Rules: []config.Rule{
			{Name: "Rule 1", Content: "Content 1"},
			{Name: "Rule 2", Content: "Content 2"},
		},
	}

	gen := NewWithBaseDir(tmpDir)
	err := gen.GenerateAllConcurrent(cfg)
	require.NoError(t, err)

	// Verify all files were created
	for i := 0; i < 15; i++ {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("output%d.md", i))
		_, err := os.Stat(filePath)
		assert.NoError(t, err, "File %d should exist", i)
	}
}

// TestComputeFileHash tests the file hash computation
func TestComputeFileHash(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Test with regular file
	regularFile := filepath.Join(tmpDir, "regular.txt")
	content := "Hello, World!"
	err := os.WriteFile(regularFile, []byte(content), 0o644)
	require.NoError(t, err)

	hash, err := computeFileHash(regularFile)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Verify hash is consistent
	hash2, err := computeFileHash(regularFile)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)

	// Test with non-existent file
	_, err = computeFileHash(filepath.Join(tmpDir, "nonexistent.txt"))
	assert.Error(t, err)

	// Test with large file (>1MB)
	largeFile := filepath.Join(tmpDir, "large.txt")
	largeContent := strings.Repeat("a", 1024*1024+100) // Just over 1MB
	err = os.WriteFile(largeFile, []byte(largeContent), 0o644)
	require.NoError(t, err)

	largeHash, err := computeFileHash(largeFile)
	require.NoError(t, err)
	assert.NotEmpty(t, largeHash)
}

// TestComputeContentHashPooled tests the pooled hash function
func TestComputeContentHashPooled(t *testing.T) {
	t.Parallel()

	content := "Test content for hashing"
	hash1 := ComputeContentHashPooled(content)
	hash2 := ComputeContentHashPooled(content)

	// Hashes should be consistent
	assert.Equal(t, hash1, hash2)
	assert.NotEmpty(t, hash1)

	// Different content should produce different hash
	hash3 := ComputeContentHashPooled("Different content")
	assert.NotEqual(t, hash1, hash3)

	// Test concurrent usage (ensure pool works correctly)
	var wg sync.WaitGroup
	hashes := make([]string, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hashes[idx] = ComputeContentHashPooled(content)
		}(i)
	}

	wg.Wait()

	// All hashes should be the same
	for i := 1; i < 100; i++ {
		assert.Equal(t, hashes[0], hashes[i], "Hash %d should match", i)
	}
}

// TestPreviewAll tests the PreviewAll function
func TestPreviewAll(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Preview Test",
			Version:     "1.0.0",
			Description: "Testing preview",
		},
		Outputs: []config.Output{
			{Path: "output1.md"},
			{Path: "output2.md"},
			{Path: "agents/", Type: "agent"},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
		},
		Agents: []config.Agent{
			{Name: "test-agent", Description: "Test agent"},
		},
	}

	gen := New()
	previews, err := gen.PreviewAll(cfg)
	require.NoError(t, err)

	assert.Len(t, previews, 3)

	// Check that all previews have content
	for path, content := range previews {
		assert.NotEmpty(t, content, "Preview for %s should have content", path)
		if strings.HasSuffix(path, ".md") {
			assert.Contains(t, content, "Preview Test")
		}
	}
}

// TestWriteSingleFile_ErrorPaths tests error handling in writeSingleFile
func TestWriteSingleFile_ErrorPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gen := NewWithBaseDir(tmpDir)

	data := &templates.TemplateData{
		ProjectName: "Test",
		Rules:       []config.Rule{{Name: "Test", Content: "Content"}},
	}

	// Test with invalid template reference
	output := &config.Output{
		Path:     "test.md",
		Template: "@/nonexistent/template.tmpl",
	}
	err := gen.writeSingleFile(output, data)
	assert.Error(t, err)
	assert.Error(t, err)

	// Test with invalid inline template
	output = &config.Output{
		Path:     "test.md",
		Template: "{{.Invalid}",
	}
	err = gen.writeSingleFile(output, data)
	assert.Error(t, err)
	assert.Error(t, err)
}

// TestWriteDirectoryOutput_ErrorPaths tests error handling in writeDirectoryOutput
func TestWriteDirectoryOutput_ErrorPaths(t *testing.T) {
	t.Parallel()

	// Test with read-only directory (permission denied)
	if os.Getuid() != 0 { // Skip if running as root
		tmpDir := t.TempDir()
		readOnlyDir := filepath.Join(tmpDir, "readonly")
		err := os.Mkdir(readOnlyDir, 0o555) // Read-only directory
		require.NoError(t, err)

		gen := NewWithBaseDir(readOnlyDir)
		output := &config.Output{
			Path: "subdir/",
			Type: "agent",
		}
		data := &templates.TemplateData{
			Agents: []config.Agent{
				{Name: "test", Description: "Test"},
			},
		}

		err = gen.writeDirectoryOutput(output, data)
		assert.Error(t, err)
		assert.Error(t, err)
	}
}

// TestWriteAgentFiles_ErrorPaths tests error handling in writeAgentFiles
func TestWriteAgentFiles_ErrorPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gen := NewWithBaseDir(tmpDir)

	// Test with agent that has invalid template
	output := &config.Output{
		Path: "agents/",
		Type: "agent",
	}
	data := &templates.TemplateData{
		Agents: []config.Agent{
			{
				Name:        "test-agent",
				Description: "Test",
				Template:    "{{.InvalidVar}",
			},
		},
	}

	err := gen.writeAgentFiles("agents/", "{name}.md", output, data)
	assert.Error(t, err)
	assert.Error(t, err)
}

// TestRenderTemplate_ErrorPaths tests error handling in renderTemplate
func TestRenderTemplate_ErrorPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gen := NewWithBaseDir(tmpDir)

	data := &templates.TemplateData{
		ProjectName: "Test",
	}

	// Test with file reference template that doesn't exist
	output := &config.Output{
		Template: "@/nonexistent/path.tmpl",
	}
	_, err := gen.renderTemplate(output, data)
	assert.Error(t, err)
	assert.Error(t, err)

	// Test with invalid inline template
	output = &config.Output{
		Template: "{{range .Invalid}}{{end}}",
	}
	_, err = gen.renderTemplate(output, data)
	assert.Error(t, err)

	// Test with file reference in read-only location
	output = &config.Output{
		Template: "@/root/protected.tmpl",
	}
	_, err = gen.renderTemplate(output, data)
	assert.Error(t, err)
}

// TestShouldWriteFile_LargeFile tests shouldWriteFile with large files
func TestShouldWriteFile_LargeFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gen := NewWithBaseDir(tmpDir)

	// Create a large file (>1MB)
	largeFile := "large.txt"
	largeContent := strings.Repeat("a", 1024*1024+100)
	fullPath := filepath.Join(tmpDir, largeFile)
	err := os.WriteFile(fullPath, []byte(largeContent), 0o644)
	require.NoError(t, err)

	// Test with same content (should not write)
	shouldWrite, err := gen.shouldWriteFile(largeFile, largeContent)
	require.NoError(t, err)
	assert.False(t, shouldWrite, "Should not write identical content")

	// Test with different content (should write)
	newContent := strings.Repeat("b", 1024*1024+100)
	shouldWrite, err = gen.shouldWriteFile(largeFile, newContent)
	require.NoError(t, err)
	assert.True(t, shouldWrite, "Should write different content")
}

// TestShouldWriteFile_Errors tests error conditions in shouldWriteFile
func TestShouldWriteFile_Errors(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gen := NewWithBaseDir(tmpDir)

	// Test with file that exists but can't be read
	if os.Getuid() != 0 { // Skip if running as root
		unreadableFile := "unreadable.txt"
		fullPath := filepath.Join(tmpDir, unreadableFile)
		err := os.WriteFile(fullPath, []byte("content"), 0o000) // No permissions
		require.NoError(t, err)

		_, err = gen.shouldWriteFile(unreadableFile, "new content")
		assert.Error(t, err)
		assert.Error(t, err)

		// Clean up
		os.Chmod(fullPath, 0o644)
	}
}

// TestWriteFile_Errors tests error conditions in writeFile
func TestWriteFile_Errors(t *testing.T) {
	t.Parallel()

	// Test with invalid path
	gen := NewWithBaseDir("/nonexistent/base/dir")
	err := gen.writeFile("test.txt", "content")
	assert.Error(t, err)
	assert.Error(t, err)
}

// TestWriteDirectoryOutput_EdgeCases tests edge cases in directory output
func TestWriteDirectoryOutput_EdgeCases(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gen := NewWithBaseDir(tmpDir)

	// Test with unknown output type (defaults to rules)
	output := &config.Output{
		Path: "output/",
		Type: "unknown",
	}
	data := &templates.TemplateData{
		ProjectName: "Test",
		Rules: []config.Rule{
			{Name: "Rule", Content: "Content"},
		},
	}

	err := gen.writeDirectoryOutput(output, data)
	require.NoError(t, err)

	// Check that rule file was created
	files, err := os.ReadDir(filepath.Join(tmpDir, "output"))
	require.NoError(t, err)
	assert.Len(t, files, 1)
}

// TestGenerateAll_IncrementalWrite tests that files are only written when changed
func TestGenerateAll_IncrementalWrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.md")

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Incremental Test",
		},
		Outputs: []config.Output{
			{Path: "output.md"},
		},
		Rules: []config.Rule{
			{Name: "Rule", Content: "Content"},
		},
	}

	gen := NewWithBaseDir(tmpDir)

	// First generation
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	// Get initial modification time
	stat1, err := os.Stat(outputFile)
	require.NoError(t, err)
	modTime1 := stat1.ModTime()

	// Wait a bit to ensure different timestamp
	time.Sleep(10 * time.Millisecond)

	// Generate again with same content
	err = gen.GenerateAll(cfg)
	require.NoError(t, err)

	// Check that file was not rewritten
	stat2, err := os.Stat(outputFile)
	require.NoError(t, err)
	modTime2 := stat2.ModTime()

	assert.Equal(t, modTime1, modTime2, "File should not be rewritten with same content")

	// Change content and generate again
	cfg.Rules[0].Content = "Different content"
	err = gen.GenerateAll(cfg)
	require.NoError(t, err)

	// Check that file was rewritten
	stat3, err := os.Stat(outputFile)
	require.NoError(t, err)
	modTime3 := stat3.ModTime()

	assert.NotEqual(t, modTime2, modTime3, "File should be rewritten with different content")
}

// TestFormatSpecifierParsing tests the formatted index/priority parsing
func TestFormatSpecifierParsing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gen := NewWithBaseDir(tmpDir)

	output := &config.Output{
		Path:         "agents/",
		Type:         "agent",
		NamingScheme: "agent-{index:04d}-{priority:02d}-{name}.md",
	}

	data := &templates.TemplateData{
		Agents: []config.Agent{
			{Name: "first", Description: "First", Priority: 5},
			{Name: "second", Description: "Second", Priority: 15},
		},
	}

	err := gen.writeAgentFiles("agents/", output.GetNamingScheme(), output, data)
	require.NoError(t, err)

	// Check generated filenames
	expectedFiles := []string{
		"agent-0001-05-first.md",
		"agent-0002-15-second.md",
	}

	for _, filename := range expectedFiles {
		filePath := filepath.Join(tmpDir, "agents", filename)
		_, err := os.Stat(filePath)
		assert.NoError(t, err, "File %s should exist", filename)
	}
}
