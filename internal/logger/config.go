package logger

// Config holds logger configuration
type Config struct {
	Level   string
	Format  string // "text" or "json"
	NoColor bool
	Debug   bool
	Verbose bool
	Quiet   bool
}
