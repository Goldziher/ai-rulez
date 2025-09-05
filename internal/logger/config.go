package logger

type Config struct {
	Level   string
	Format  string
	NoColor bool
	Debug   bool
	Verbose bool
	Quiet   bool
}
