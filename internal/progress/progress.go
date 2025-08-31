package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

var (
	// Global quiet mode flag
	quiet bool
	mu    sync.RWMutex
)

// SetQuiet sets the global quiet mode
func SetQuiet(q bool) {
	mu.Lock()
	defer mu.Unlock()
	quiet = q
}

// IsQuiet returns whether quiet mode is enabled
func IsQuiet() bool {
	mu.RLock()
	defer mu.RUnlock()
	return quiet
}

// Bar represents a progress bar that respects quiet mode
type Bar struct {
	bar    *progressbar.ProgressBar
	quiet  bool
	prefix string
}

// New creates a new progress bar with the given max value and description
func New(maximum int, description string) *Bar {
	if IsQuiet() {
		return &Bar{
			quiet:  true,
			prefix: description,
		}
	}

	bar := progressbar.NewOptions(maximum,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(false),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)

	return &Bar{
		bar:    bar,
		quiet:  false,
		prefix: description,
	}
}

// NewBytes creates a progress bar for byte operations (downloads/uploads)
func NewBytes(maxBytes int64, description string) *Bar {
	if IsQuiet() {
		return &Bar{
			quiet:  true,
			prefix: description,
		}
	}

	bar := progressbar.NewOptions64(maxBytes,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("B"),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionShowElapsedTimeOnFinish(),
	)

	return &Bar{
		bar:    bar,
		quiet:  false,
		prefix: description,
	}
}

// NewSpinner creates an indeterminate progress indicator
func NewSpinner(description string) *Bar {
	if IsQuiet() {
		return &Bar{
			quiet:  true,
			prefix: description,
		}
	}

	bar := progressbar.NewOptions(-1,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionClearOnFinish(),
	)

	return &Bar{
		bar:    bar,
		quiet:  false,
		prefix: description,
	}
}

// Add increments the progress bar by the given amount
func (b *Bar) Add(delta int) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Add(delta)
}

// Add64 increments the progress bar by the given amount (for byte operations)
func (b *Bar) Add64(delta int64) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Add64(delta)
}

// Set sets the current progress value
func (b *Bar) Set(value int) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Set(value)
}

// Set64 sets the current progress value (for byte operations)
func (b *Bar) Set64(value int64) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Set64(value)
}

// Finish completes the progress bar
func (b *Bar) Finish() error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Finish()
}

// Clear clears the progress bar from the terminal
func (b *Bar) Clear() error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Clear()
}

// ChangeDescription updates the progress bar description
func (b *Bar) ChangeDescription(description string) {
	b.prefix = description
	if !b.quiet && b.bar != nil {
		b.bar.Describe(description)
	}
}

// Write implements io.Writer for download progress
func (b *Bar) Write(p []byte) (n int, err error) {
	n = len(p)
	if !b.quiet && b.bar != nil {
		//nolint:errcheck // Progress bar errors are non-critical
		_ = b.bar.Add(n)
	}
	return n, nil
}

// NewReader wraps an io.Reader with progress tracking
func NewReader(reader io.Reader, size int64, description string) io.Reader {
	if IsQuiet() {
		return reader
	}

	bar := NewBytes(size, description)
	return io.TeeReader(reader, bar)
}

// NewWriter wraps an io.Writer with progress tracking
func NewWriter(writer io.Writer, size int64, description string) io.Writer {
	if IsQuiet() {
		return writer
	}

	bar := NewBytes(size, description)
	return io.MultiWriter(writer, bar)
}

// PrintIfNotQuiet prints a message only if not in quiet mode
func PrintIfNotQuiet(format string, args ...interface{}) {
	if !IsQuiet() {
		fmt.Printf(format, args...)
	}
}

// PrintlnIfNotQuiet prints a line only if not in quiet mode
func PrintlnIfNotQuiet(message string) {
	if !IsQuiet() {
		fmt.Println(message)
	}
}

// FileCounter tracks progress for multiple file operations
type FileCounter struct {
	bar         *Bar
	total       int
	current     int
	currentFile string
	mu          sync.Mutex
}

// NewFileCounter creates a counter for tracking file operations
func NewFileCounter(totalFiles int, description string) *FileCounter {
	return &FileCounter{
		bar:   New(totalFiles, description),
		total: totalFiles,
	}
}

// StartFile marks the beginning of processing a file
func (fc *FileCounter) StartFile(filename string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.currentFile = filename
	if !fc.bar.quiet {
		desc := fmt.Sprintf("Processing %s (%d/%d)", filename, fc.current+1, fc.total)
		fc.bar.ChangeDescription(desc)
	}
}

// FinishFile marks the completion of processing a file
func (fc *FileCounter) FinishFile() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.current++
	//nolint:errcheck // Progress bar errors are non-critical
	_ = fc.bar.Add(1)
	fc.currentFile = ""
}

// Finish completes the file counter
func (fc *FileCounter) Finish() {
	//nolint:errcheck // Progress bar errors are non-critical
	_ = fc.bar.Finish()
	if !fc.bar.quiet {
		PrintlnIfNotQuiet(fmt.Sprintf("✅ Processed %d file(s)", fc.total))
	}
}

// Error marks an error for the current file
func (fc *FileCounter) Error(err error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if !fc.bar.quiet && fc.currentFile != "" {
		// Clear the progress bar temporarily to show error
		//nolint:errcheck // Progress bar errors are non-critical
		_ = fc.bar.Clear()
		fmt.Fprintf(os.Stderr, "❌ Error processing %s: %v\n", fc.currentFile, err)
	}
	fc.current++
	//nolint:errcheck // Progress bar errors are non-critical
	_ = fc.bar.Add(1)
}
