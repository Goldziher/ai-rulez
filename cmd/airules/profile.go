package airules

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/spf13/cobra"
)

var (
	cpuprofile string
	memprofile string
)

// profileCmd represents the profile command
var profileCmd = &cobra.Command{
	Use:    "profile",
	Hidden: true, // Hidden command for development use
	Short:  "Run with profiling enabled",
	Long:   `Run airules commands with CPU and memory profiling enabled for performance analysis.`,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		if cpuprofile != "" {
			f, err := os.Create(cpuprofile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create CPU profile: %v\n", err)
				os.Exit(1)
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to start CPU profile: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "CPU profiling enabled, writing to %s\n", cpuprofile)
		}
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		if cpuprofile != "" {
			pprof.StopCPUProfile()
			fmt.Fprintf(os.Stderr, "CPU profile written to %s\n", cpuprofile)
		}

		if memprofile != "" {
			f, err := os.Create(memprofile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create memory profile: %v\n", err)
				return
			}
			defer func() { _ = f.Close() }()

			runtime.GC() // get up-to-date statistics for memory profiling
			if err := pprof.WriteHeapProfile(f); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write memory profile: %v\n", err)
				return
			}
			fmt.Fprintf(os.Stderr, "Memory profile written to %s\n", memprofile)
		}
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)

	// Add subcommands that mirror the main commands
	profileGenerate := *generateCmd
	profileValidate := *validateCmd

	profileCmd.AddCommand(&profileGenerate)
	profileCmd.AddCommand(&profileValidate)

	// Profile flags
	profileCmd.PersistentFlags().StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	profileCmd.PersistentFlags().StringVar(&memprofile, "memprofile", "", "write memory profile to file")
}
