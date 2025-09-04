package crud

import (
	"fmt"
	"os"

	"github.com/samber/oops"
)

// FmtError formats an error for display
func FmtError(err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "\033[31mError:\033[0m %v\n", err)

	if oopsErr, ok := oops.AsOops(err); ok {
		ctx := oopsErr.Context()

		if validationErrors, ok := ctx["errors"].([]string); ok && len(validationErrors) > 0 {
			fmt.Fprintf(os.Stderr, "\n\033[31mValidation Errors:\033[0m\n")
			for _, errMsg := range validationErrors {
				fmt.Fprintf(os.Stderr, "  %s\n", errMsg)
			}
		}

		if hint := oopsErr.Hint(); hint != "" {
			fmt.Fprintf(os.Stderr, "\n\033[36mHint:\033[0m %s\n", hint)
		}
	}
}
