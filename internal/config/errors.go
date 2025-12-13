package config

import "errors"

// V3 Configuration errors
var (
	ErrInvalidVersion        = errors.New("invalid version: must be '3.0'")
	ErrMissingName           = errors.New("missing required field: name")
	ErrInvalidDefaultProfile = errors.New("default profile does not exist in profiles")
	ErrInvalidPresetV3       = errors.New("invalid preset configuration")
	ErrNoContent             = errors.New("no content loaded")
)
