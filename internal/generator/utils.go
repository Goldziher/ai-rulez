package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

var hashPool = sync.Pool{
	New: func() interface{} {
		return sha256.New()
	},
}

func (g *Generator) shouldWriteFile(filePath, newContent string) (bool, error) {
	fullPath := filepath.Join(g.baseDir, filePath)

	stat, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, oops.
			With("path", fullPath).
			With("operation", "check file status").
			Hint("Check if you have read permissions for the file").
			Wrapf(err, "failed to read file status")
	}

	if stat.Size() < 1024*1024 {
		existingContent, err := os.ReadFile(fullPath)
		if err != nil {
			return false, oops.
				With("path", fullPath).
				With("operation", "read existing file for comparison").
				Wrapf(err, "failed to read existing file")
		}
		existingHash := ComputeContentHashPooled(string(existingContent))
		newHash := ComputeContentHashPooled(newContent)
		return existingHash != newHash, nil
	}

	existingHash, err := computeFileHash(fullPath)
	if err != nil {
		return false, oops.
			With("path", fullPath).
			With("operation", "compute file hash").
			Hint("Check if the file is not locked by another process").
			Wrapf(err, "failed to compute file hash")
	}

	newHash := ComputeContentHashPooled(newContent)
	return existingHash != newHash, nil
}

func (g *Generator) writeFile(filePath, content string) error {
	fullPath := filepath.Join(g.baseDir, filePath)

	outputDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return oops.
			With("path", outputDir).
			With("operation", "create output directory").
			Hint("Check if you have write permissions for the parent directory").
			Wrapf(err, "failed to create output directory")
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return oops.
			With("path", fullPath).
			Wrapf(err, "failed to write file")
	}

	return nil
}

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

// ComputeContentHashPooled computes SHA256 hash using a pooled hasher
func ComputeContentHashPooled(content string) string {
	h := hashPool.Get().(hash.Hash)
	defer func() {
		h.Reset()
		hashPool.Put(h)
	}()

	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

func (*Generator) findOutputConfig(outputs []config.Output, outputFile string) *config.Output {
	for _, output := range outputs {
		if output.GetPath() == outputFile {
			return &output
		}
	}
	return nil
}

func (*Generator) getOutputFileNames(outputs []config.Output) []string {
	names := make([]string, len(outputs))
	for i, output := range outputs {
		names[i] = output.GetPath()
	}
	return names
}
