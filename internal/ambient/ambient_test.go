package ambient

import (
	"context"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestMapEnv(t *testing.T) {
	tests := []struct {
		name     string
		env      Env
		key      string
		want     string
		wantHome string
		homeErr  bool
	}{
		{"set variable", MapEnv{Vars: map[string]string{"A": "1"}, Home: "/h"}, "A", "1", "/h", false},
		{"missing variable", MapEnv{Home: "/h"}, "A", "", "/h", false},
		{"no home", MapEnv{}, "A", "", "", true},
		{"nil env is the real one", nil, "AMBIENT_TEST_SURELY_UNSET", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange and Act
			got := Getenv(tt.env, tt.key)
			// Assert
			if got != tt.want {
				t.Fatalf("Getenv = %q, want %q", got, tt.want)
			}
			if tt.env == nil {
				return
			}
			home, err := tt.env.UserHomeDir()
			if (err != nil) != tt.homeErr || home != tt.wantHome {
				t.Fatalf("UserHomeDir = %q, %v", home, err)
			}
		})
	}
}

func TestExpandUsesTheInjectedEnv(t *testing.T) {
	// Arrange
	env := MapEnv{Vars: map[string]string{"NAME": "ai", "EMPTY": ""}}
	// Act
	got := Expand(env, "${NAME}-$NAME-${UNSET}-${EMPTY}")
	// Assert
	if got != "ai-ai--" {
		t.Fatalf("Expand = %q", got)
	}
}

func TestClock(t *testing.T) {
	// Arrange
	pinned := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	// Act and Assert
	if got := Fixed(pinned).Now(); !got.Equal(pinned) {
		t.Fatalf("Fixed clock = %v", got)
	}
	var zero Clock
	if time.Since(zero.Now()) > time.Minute {
		t.Fatal("the zero clock must be the wall clock")
	}
}

func TestHostContextCarriesRunnerAndLogger(t *testing.T) {
	// Arrange
	fake := &runner.Fake{}
	host := Host{Runner: fake, Log: logger.Discard(), Env: MapEnv{Home: "/h"}}
	// Act
	ctx := WithContext(context.Background(), host)
	// Assert
	if runner.FromContext(ctx) != runner.Runner(fake) {
		t.Fatal("the runner did not travel with the context")
	}
	if got, err := FromContext(ctx).Home(); err != nil || got != "/h" {
		t.Fatalf("Home = %q, %v", got, err)
	}
	if FromContext(context.Background()).Runner != nil {
		t.Fatal("an empty context carries the zero Host")
	}
}
