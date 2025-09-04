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
	quiet bool
	mu    sync.RWMutex
)

func SetQuiet(q bool) {
	mu.Lock()
	defer mu.Unlock()
	quiet = q
}

func IsQuiet() bool {
	mu.RLock()
	defer mu.RUnlock()
	return quiet
}

type Bar struct {
	bar    *progressbar.ProgressBar
	quiet  bool
	prefix string
}

var defaultTheme = progressbar.Theme{
	Saucer:        "[green]=[reset]",
	SaucerHead:    "[green]>[reset]",
	SaucerPadding: " ",
	BarStart:      "[",
	BarEnd:        "]",
}

func commonOptions(description string) []progressbar.Option {
	return []progressbar.Option{
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetTheme(defaultTheme),
		progressbar.OptionClearOnFinish(),
	}
}

func createQuietBar(description string) *Bar {
	return &Bar{
		quiet:  true,
		prefix: description,
	}
}

func New(maximum int, description string) *Bar {
	if IsQuiet() {
		return createQuietBar(description)
	}

	opts := append(commonOptions(description),
		progressbar.OptionShowBytes(false),
		progressbar.OptionShowCount(),
	)

	bar := progressbar.NewOptions(maximum, opts...)

	return &Bar{
		bar:    bar,
		quiet:  false,
		prefix: description,
	}
}

func NewBytes(maxBytes int64, description string) *Bar {
	if IsQuiet() {
		return createQuietBar(description)
	}

	opts := append(commonOptions(description),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("B"),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowElapsedTimeOnFinish(),
	)

	bar := progressbar.NewOptions64(maxBytes, opts...)

	return &Bar{
		bar:    bar,
		quiet:  false,
		prefix: description,
	}
}

func NewSpinner(description string) *Bar {
	if IsQuiet() {
		return createQuietBar(description)
	}

	opts := []progressbar.Option{
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionClearOnFinish(),
	}

	bar := progressbar.NewOptions(-1, opts...)

	return &Bar{
		bar:    bar,
		quiet:  false,
		prefix: description,
	}
}

func (b *Bar) Add(delta int) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Add(delta)
}

func (b *Bar) Add64(delta int64) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Add64(delta)
}

func (b *Bar) Set(value int) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Set(value)
}

func (b *Bar) Set64(value int64) error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Set64(value)
}

func (b *Bar) Finish() error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Finish()
}

func (b *Bar) Clear() error {
	if b.quiet || b.bar == nil {
		return nil
	}
	return b.bar.Clear()
}

func (b *Bar) ChangeDescription(description string) {
	b.prefix = description
	if !b.quiet && b.bar != nil {
		b.bar.Describe(description)
	}
}

func (b *Bar) Write(p []byte) (n int, err error) {
	n = len(p)
	if !b.quiet && b.bar != nil {
		//nolint:errcheck // Progress bar errors are non-critical
		_ = b.bar.Add(n)
	}
	return n, nil
}

func NewReader(reader io.Reader, size int64, description string) io.Reader {
	if IsQuiet() {
		return reader
	}

	bar := NewBytes(size, description)
	return io.TeeReader(reader, bar)
}

func NewWriter(writer io.Writer, size int64, description string) io.Writer {
	if IsQuiet() {
		return writer
	}

	bar := NewBytes(size, description)
	return io.MultiWriter(writer, bar)
}

func PrintIfNotQuiet(format string, args ...interface{}) {
	if !IsQuiet() {
		fmt.Printf(format, args...)
	}
}

func PrintlnIfNotQuiet(message string) {
	if !IsQuiet() {
		fmt.Println(message)
	}
}

type FileCounter struct {
	bar         *Bar
	total       int
	current     int
	currentFile string
	mu          sync.Mutex
}

func NewFileCounter(totalFiles int, description string) *FileCounter {
	return &FileCounter{
		bar:   New(totalFiles, description),
		total: totalFiles,
	}
}

func (fc *FileCounter) StartFile(filename string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.currentFile = filename
	if !fc.bar.quiet {
		desc := fmt.Sprintf("Processing %s (%d/%d)", filename, fc.current+1, fc.total)
		fc.bar.ChangeDescription(desc)
	}
}

func (fc *FileCounter) FinishFile() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.current++
	//nolint:errcheck // Progress bar errors are non-critical
	_ = fc.bar.Add(1)
	fc.currentFile = ""
}

func (fc *FileCounter) Finish() {
	//nolint:errcheck // Progress bar errors are non-critical
	_ = fc.bar.Finish()
	if !fc.bar.quiet {
		PrintlnIfNotQuiet(fmt.Sprintf("✅ Processed %d file(s)", fc.total))
	}
}

func (fc *FileCounter) Error(err error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if !fc.bar.quiet && fc.currentFile != "" {
		//nolint:errcheck // Progress bar errors are non-critical
		_ = fc.bar.Clear()
		fmt.Fprintf(os.Stderr, "❌ Error processing %s: %v\n", fc.currentFile, err)
	}
	fc.current++
	//nolint:errcheck // Progress bar errors are non-critical
	_ = fc.bar.Add(1)
}
