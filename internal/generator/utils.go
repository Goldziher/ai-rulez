package generator

import (
	"bufio"
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

	return writeFileBuffered(fullPath, content)
}

func writeFileBuffered(filePath, content string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return oops.
			With("path", filePath).
			Hint("Check write permissions and available disk space").
			Wrapf(err, "create file")
	}
	defer func() { _ = file.Close() }()

	bufferSize := 8192
	if len(content) > 64*1024 {
		bufferSize = 32768
	}

	writer := bufio.NewWriterSize(file, bufferSize)
	defer func() { _ = writer.Flush() }()

	if _, err := writer.WriteString(content); err != nil {
		return oops.
			With("path", filePath).
			Hint("Check available disk space").
			Wrapf(err, "write content")
	}

	if err := writer.Flush(); err != nil {
		return oops.
			With("path", filePath).
			Hint("Check available disk space").
			Wrapf(err, "flush content")
	}

	return nil
}

func computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReaderSize(file, 32768)

	h := hashPool.Get().(hash.Hash)
	defer func() {
		h.Reset()
		hashPool.Put(h)
	}()

	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

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
		if output.Path == outputFile {
			return &output
		}
	}
	return nil
}

func (*Generator) getOutputFileNames(outputs []config.Output) []string {
	names := make([]string, len(outputs))
	for i, output := range outputs {
		names[i] = output.Path
	}
	return names
}
