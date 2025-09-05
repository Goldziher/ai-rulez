package progress

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQuietMode(t *testing.T) {
	t.Run("set and get quiet mode", func(t *testing.T) {
		SetQuiet(false)
		assert.False(t, IsQuiet())

		SetQuiet(true)
		assert.True(t, IsQuiet())

		SetQuiet(false)
		assert.False(t, IsQuiet())
	})

	t.Run("concurrent access", func(t *testing.T) {
		SetQuiet(false)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(val bool) {
				defer wg.Done()
				SetQuiet(val)
				_ = IsQuiet()
			}(i%2 == 0)
		}
		wg.Wait()

		_ = IsQuiet()
	})
}

func TestProgressBar(t *testing.T) {
	t.Run("new progress bar respects quiet mode", func(t *testing.T) {
		SetQuiet(false)
		bar := New(100, "Test progress")
		assert.NotNil(t, bar)
		assert.False(t, bar.quiet)
		assert.NotNil(t, bar.bar)

		SetQuiet(true)
		bar = New(100, "Test progress")
		assert.NotNil(t, bar)
		assert.True(t, bar.quiet)
		assert.Nil(t, bar.bar)

		SetQuiet(false)
	})

	t.Run("progress bar operations in quiet mode", func(t *testing.T) {
		SetQuiet(true)
		bar := New(100, "Test progress")

		assert.NoError(t, bar.Add(10))
		assert.NoError(t, bar.Set(50))
		assert.NoError(t, bar.Finish())
		assert.NoError(t, bar.Clear())

		bar.ChangeDescription("New description")

		SetQuiet(false)
	})

	t.Run("progress bar operations in normal mode", func(t *testing.T) {
		SetQuiet(false)
		bar := New(100, "Test progress")

		assert.NoError(t, bar.Add(10))
		assert.NoError(t, bar.Set(50))

		bar.ChangeDescription("Updated description")
		assert.Equal(t, "Updated description", bar.prefix)

		assert.NoError(t, bar.Finish())
	})
}

func TestProgressBarBytes(t *testing.T) {
	t.Run("bytes progress bar", func(t *testing.T) {
		SetQuiet(false)
		bar := NewBytes(1024*1024, "Download test")
		assert.NotNil(t, bar)
		assert.NotNil(t, bar.bar)

		assert.NoError(t, bar.Add64(1024))
		assert.NoError(t, bar.Set64(512*1024))
		assert.NoError(t, bar.Finish())
	})

	t.Run("bytes progress in quiet mode", func(t *testing.T) {
		SetQuiet(true)
		bar := NewBytes(1024*1024, "Download test")
		assert.NotNil(t, bar)
		assert.Nil(t, bar.bar)

		assert.NoError(t, bar.Add64(1024))
		assert.NoError(t, bar.Set64(512*1024))

		SetQuiet(false)
	})
}

func TestSpinner(t *testing.T) {
	t.Run("spinner creation", func(t *testing.T) {
		SetQuiet(false)
		spinner := NewSpinner("Processing...")
		assert.NotNil(t, spinner)
		assert.NotNil(t, spinner.bar)
		assert.NoError(t, spinner.Add(1))
		assert.NoError(t, spinner.Finish())
	})

	t.Run("spinner in quiet mode", func(t *testing.T) {
		SetQuiet(true)
		spinner := NewSpinner("Processing...")
		assert.NotNil(t, spinner)
		assert.Nil(t, spinner.bar)
		assert.NoError(t, spinner.Add(1))
		assert.NoError(t, spinner.Finish())

		SetQuiet(false)
	})
}

func TestProgressBarWrite(t *testing.T) {
	t.Run("progress bar as io.Writer", func(t *testing.T) {
		SetQuiet(false)
		bar := NewBytes(1024, "Write test")

		data := []byte("test data")
		n, err := bar.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
	})

	t.Run("write in quiet mode", func(t *testing.T) {
		SetQuiet(true)
		bar := NewBytes(1024, "Write test")

		data := []byte("test data")
		n, err := bar.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)

		SetQuiet(false)
	})
}

func TestIOWrappers(t *testing.T) {
	t.Run("reader wrapper", func(t *testing.T) {
		SetQuiet(false)

		data := []byte("test data for reading")
		reader := bytes.NewReader(data)

		wrappedReader := NewReader(reader, int64(len(data)), "Reading test")
		assert.NotNil(t, wrappedReader)

		result, err := io.ReadAll(wrappedReader)
		assert.NoError(t, err)
		assert.Equal(t, data, result)
	})

	t.Run("reader wrapper in quiet mode", func(t *testing.T) {
		SetQuiet(true)

		data := []byte("test data for reading")
		reader := bytes.NewReader(data)

		wrappedReader := NewReader(reader, int64(len(data)), "Reading test")
		assert.NotNil(t, wrappedReader)
		assert.Equal(t, reader, wrappedReader)

		SetQuiet(false)
	})

	t.Run("writer wrapper", func(t *testing.T) {
		SetQuiet(false)

		var buf bytes.Buffer
		wrappedWriter := NewWriter(&buf, 1024, "Writing test")
		assert.NotNil(t, wrappedWriter)

		data := []byte("test data for writing")
		n, err := wrappedWriter.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, buf.Bytes())
	})

	t.Run("writer wrapper in quiet mode", func(t *testing.T) {
		SetQuiet(true)

		var buf bytes.Buffer
		wrappedWriter := NewWriter(&buf, 1024, "Writing test")
		assert.NotNil(t, wrappedWriter)
		assert.Equal(t, &buf, wrappedWriter)

		SetQuiet(false)
	})
}

func TestPrintFunctions(t *testing.T) {
	t.Run("print if not quiet", func(t *testing.T) {
		old := captureOutput()
		defer restoreOutput(old)

		SetQuiet(false)
		PrintIfNotQuiet("Test %s %d\n", "message", 123)

		SetQuiet(true)
		PrintIfNotQuiet("Test %s %d\n", "message", 123)

		SetQuiet(false)
	})

	t.Run("println if not quiet", func(t *testing.T) {
		old := captureOutput()
		defer restoreOutput(old)

		SetQuiet(false)
		PrintlnIfNotQuiet("Test message")

		SetQuiet(true)
		PrintlnIfNotQuiet("Test message")

		SetQuiet(false)
	})
}

func TestFileCounter(t *testing.T) {
	t.Run("file counter basic operations", func(t *testing.T) {
		SetQuiet(false)

		fc := NewFileCounter(5, "Processing files")
		assert.NotNil(t, fc)
		assert.Equal(t, 5, fc.total)
		assert.Equal(t, 0, fc.current)

		fc.StartFile("file1.txt")
		assert.Equal(t, "file1.txt", fc.currentFile)
		fc.FinishFile()
		assert.Equal(t, 1, fc.current)
		assert.Equal(t, "", fc.currentFile)

		fc.StartFile("file2.txt")
		fc.FinishFile()
		assert.Equal(t, 2, fc.current)

		fc.Finish()
	})

	t.Run("file counter with errors", func(t *testing.T) {
		SetQuiet(false)

		fc := NewFileCounter(3, "Processing files")

		fc.StartFile("file1.txt")
		fc.FinishFile()

		fc.StartFile("file2.txt")
		fc.Error(assert.AnError)
		assert.Equal(t, 2, fc.current)

		fc.StartFile("file3.txt")
		fc.FinishFile()

		fc.Finish()
		assert.Equal(t, 3, fc.current)
	})

	t.Run("file counter in quiet mode", func(t *testing.T) {
		SetQuiet(true)

		fc := NewFileCounter(3, "Processing files")
		assert.NotNil(t, fc)
		assert.True(t, fc.bar.quiet)

		fc.StartFile("file1.txt")
		fc.FinishFile()

		fc.StartFile("file2.txt")
		fc.Error(assert.AnError)

		fc.StartFile("file3.txt")
		fc.FinishFile()

		fc.Finish()

		SetQuiet(false)
	})

	t.Run("file counter concurrent access", func(t *testing.T) {
		SetQuiet(false)

		fc := NewFileCounter(100, "Concurrent processing")

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				filename := strings.Repeat("file", idx) + ".txt"
				fc.StartFile(filename)
				time.Sleep(time.Millisecond)
				if idx%3 == 0 {
					fc.Error(assert.AnError)
				} else {
					fc.FinishFile()
				}
			}(i)
		}

		wg.Wait()
		fc.Finish()

		assert.Equal(t, 10, fc.current)
	})
}

func TestProgressBarEdgeCases(t *testing.T) {
	t.Run("nil operations", func(t *testing.T) {
		bar := &Bar{
			quiet: true,
			bar:   nil,
		}

		assert.NoError(t, bar.Add(10))
		assert.NoError(t, bar.Add64(100))
		assert.NoError(t, bar.Set(50))
		assert.NoError(t, bar.Set64(500))
		assert.NoError(t, bar.Finish())
		assert.NoError(t, bar.Clear())

		n, err := bar.Write([]byte("test"))
		assert.NoError(t, err)
		assert.Equal(t, 4, n)
	})

	t.Run("change description on nil bar", func(t *testing.T) {
		bar := &Bar{
			quiet:  true,
			bar:    nil,
			prefix: "old",
		}

		bar.ChangeDescription("new")
		assert.Equal(t, "new", bar.prefix)
	})

	t.Run("zero max value", func(t *testing.T) {
		SetQuiet(false)

		bar := New(0, "Zero max")
		assert.NotNil(t, bar)

		_ = bar.Add(1)
		_ = bar.Finish()
	})

	t.Run("negative max value for spinner", func(t *testing.T) {
		SetQuiet(false)

		spinner := NewSpinner("Indeterminate")
		assert.NotNil(t, spinner)

		assert.NoError(t, spinner.Add(1))
		assert.NoError(t, spinner.Add(1))
		assert.NoError(t, spinner.Finish())
	})
}

func captureOutput() *bytes.Buffer {
	return &bytes.Buffer{}
}

func restoreOutput(old *bytes.Buffer) {
}

func BenchmarkProgressBar(b *testing.B) {
	SetQuiet(false)

	b.Run("Add operations", func(b *testing.B) {
		bar := New(b.N, "Benchmark")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			bar.Add(1)
		}
		bar.Finish()
	})

	b.Run("Add operations quiet mode", func(b *testing.B) {
		SetQuiet(true)
		bar := New(b.N, "Benchmark")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			bar.Add(1)
		}
		bar.Finish()
		SetQuiet(false)
	})

	b.Run("Write operations", func(b *testing.B) {
		SetQuiet(false)
		bar := NewBytes(int64(b.N*100), "Write benchmark")
		data := make([]byte, 100)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			bar.Write(data)
		}
		bar.Finish()
	})
}

func ExampleNew() {
	bar := New(100, "Processing items")

	for i := 0; i < 100; i++ {
		bar.Add(1)
	}

	bar.Finish()
}

func ExampleNewBytes() {
	bar := NewBytes(10*1024*1024, "Downloading file")

	for i := 0; i < 10; i++ {
		bar.Add64(1024 * 1024)
	}

	bar.Finish()
}

func ExampleFileCounter() {
	files := []string{"file1.txt", "file2.txt", "file3.txt"}
	fc := NewFileCounter(len(files), "Processing files")

	for _, file := range files {
		fc.StartFile(file)
		fc.FinishFile()
	}

	fc.Finish()
}
