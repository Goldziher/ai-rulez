package ard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveQueries(t *testing.T) {
	long := strings.Repeat("word ", 50)
	tests := []struct {
		name string
		src  QuerySources
		want []string
	}{
		{name: "nothing", src: QuerySources{}, want: nil},
		{
			name: "explicit first, then eval prompts, then triggers",
			src: QuerySources{
				Explicit:    []string{"write release notes"},
				EvalPrompts: []string{"Draft the changelog for v2"},
				Triggers:    []string{"release notes", "changelog"},
			},
			want: []string{"write release notes", "Draft the changelog for v2", "release notes", "changelog"},
		},
		{
			name: "triggers only",
			src:  QuerySources{Triggers: []string{"deploy", "ship it"}},
			want: []string{"deploy", "ship it"},
		},
		{
			name: "whitespace collapsed and case-insensitive duplicates dropped",
			src: QuerySources{
				Explicit: []string{"  deploy\n the   app "},
				Triggers: []string{"Deploy the app", "", "   "},
			},
			want: []string{"deploy the app"},
		},
		{
			name: "long eval prompts skipped",
			src:  QuerySources{EvalPrompts: []string{long, "review my diff"}},
			want: []string{"review my diff"},
		},
		{
			name: "capped at five",
			src: QuerySources{
				Explicit: []string{"a", "b", "c"},
				Triggers: []string{"d", "e", "f", "g"},
			},
			want: []string{"a", "b", "c", "d", "e"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := DeriveQueries(tt.src)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
