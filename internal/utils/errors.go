package utils

import (
	"errors"
	"fmt"
	"strings"
)

func LogIfErr(err error) {
	_ = err
}

func WrapError(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "does not exist") ||
		strings.Contains(errStr, "no such")
}

func FirstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func CollectErrors(errs []error) error {
	nonNilErrors := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			nonNilErrors = append(nonNilErrors, err)
		}
	}

	switch len(nonNilErrors) {
	case 0:
		return nil
	case 1:
		return nonNilErrors[0]
	default:
		return errors.Join(nonNilErrors...)
	}
}

type StructuredError struct {
	Err       error
	Context   string
	Component string
	Hint      string
	Details   map[string]interface{}
}

func (e *StructuredError) Error() string {
	var builder strings.Builder

	if e.Context != "" {
		builder.WriteString(e.Context)
		builder.WriteString(": ")
	}

	if e.Err != nil {
		builder.WriteString(e.Err.Error())
	}

	if e.Component != "" {
		builder.WriteString(" (component: ")
		builder.WriteString(e.Component)
		builder.WriteString(")")
	}

	if e.Hint != "" {
		builder.WriteString("\nHint: ")
		builder.WriteString(e.Hint)
	}

	return builder.String()
}

func (e *StructuredError) Unwrap() error {
	return e.Err
}

func NewStructuredError(err error, component, context string) *StructuredError {
	return &StructuredError{
		Err:       err,
		Context:   context,
		Component: component,
		Details:   make(map[string]interface{}),
	}
}

func (e *StructuredError) WithHint(hint string) *StructuredError {
	e.Hint = hint
	return e
}

func (e *StructuredError) WithDetail(key string, value interface{}) *StructuredError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

func WrapWithContext(err error, component, context string) error {
	if err == nil {
		return nil
	}
	return NewStructuredError(err, component, context)
}

func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	temporaryIndicators := []string{
		"timeout", "connection refused", "connection reset",
		"temporary failure", "resource temporarily unavailable",
		"file is locked", "permission denied",
	}

	for _, indicator := range temporaryIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}

func IsConfigurationError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	configIndicators := []string{
		"config", "configuration", "invalid", "missing",
		"required", "schema", "validation", "parse",
	}

	for _, indicator := range configIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}
