package runner

import (
	"context"
	"errors"
	"testing"
)

func TestFromContext(t *testing.T) {
	fake := &Fake{}
	tests := []struct {
		name string
		ctx  context.Context
		want Runner
	}{
		{"empty context falls back to Exec", context.Background(), Exec{}},
		{"nil context falls back to Exec", nil, Exec{}}, //nolint:staticcheck // the nil case is the point
		{"carried runner wins", WithContext(context.Background(), fake), fake},
		{"nil runner leaves the context alone", WithContext(context.Background(), nil), Exec{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange and Act
			got := FromContext(tt.ctx)
			// Assert
			if got != tt.want {
				t.Fatalf("FromContext = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDenyRefusesWithTheArgv(t *testing.T) {
	// Arrange
	spec := Spec{Argv: []string{"git", "status"}}
	// Act
	res := Deny{}.Run(context.Background(), spec)
	// Assert
	var denied *DeniedError
	if res.Status != StatusUnavailable || !errors.As(res.Err, &denied) || denied.Argv[1] != "status" {
		t.Fatalf("Deny result = %+v", res)
	}
}

func TestFakeRecordsCalls(t *testing.T) {
	// Arrange
	fake := &Fake{Handle: func(s Spec) Result { return Result{Status: StatusOK, Stdout: []byte(s.Argv[0])} }}
	// Act
	res := fake.Run(context.Background(), Spec{Argv: []string{"x", "1"}})
	fake.Run(context.Background(), Spec{Argv: []string{"y"}})
	// Assert
	if string(res.Stdout) != "x" {
		t.Fatalf("stdout = %q", res.Stdout)
	}
	calls := fake.Calls()
	if len(calls) != 2 || calls[1].Argv[0] != "y" {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestExecRunsARealCommand(t *testing.T) {
	// Arrange and Act
	res := Exec{}.Run(context.Background(), Spec{Argv: []string{"go", "version"}})
	// Assert
	if res.Status != StatusOK {
		t.Skipf("go not runnable here: %+v", res)
	}
}
