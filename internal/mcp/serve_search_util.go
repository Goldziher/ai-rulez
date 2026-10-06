package mcp

import (
	"errors"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
)

func isNoIndex(err error) bool { return errors.Is(err, skillsearch.ErrNoIndex) }

func msDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }
