package config

import (
	"fmt"
	"strings"
)

type Priority string

const (
	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityMedium   Priority = "medium"
	PriorityLow      Priority = "low"
	PriorityMinimal  Priority = "minimal"
)

func (p Priority) ToInt() int {
	switch p {
	case PriorityCritical:
		return 10
	case PriorityHigh:
		return 8
	case PriorityMedium:
		return 5
	case PriorityLow:
		return 3
	case PriorityMinimal:
		return 1
	default:
		return 5
	}
}

func (p Priority) String() string {
	return string(p)
}

func (p Priority) IsValid() bool {
	switch p {
	case PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow, PriorityMinimal:
		return true
	default:
		return false
	}
}

func ParsePriority(value interface{}) (Priority, error) {
	if value == nil {
		return PriorityMedium, nil
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			return PriorityMedium, nil
		}
		p := Priority(strings.ToLower(v))
		if p.IsValid() {
			return p, nil
		}
		return "", fmt.Errorf("invalid priority string: %s", v)

	case int:
		return IntToPriority(v), nil

	case float64:
		return IntToPriority(int(v)), nil

	default:
		return "", fmt.Errorf("unsupported priority type: %T", value)
	}
}

func IntToPriority(i int) Priority {
	switch {
	case i >= 10:
		return PriorityCritical
	case i >= 8:
		return PriorityHigh
	case i >= 5:
		return PriorityMedium
	case i >= 3:
		return PriorityLow
	default:
		return PriorityMinimal
	}
}
