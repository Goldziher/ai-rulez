package ard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Identifier
		wantErr string
	}{
		{name: "three segments", in: "urn:air:acme.com:skills:deploy", want: Identifier{"acme.com", "skills", "deploy"}},
		{name: "no namespace (ADR-0007)", in: "urn:air:acme.com:assistant", want: Identifier{"acme.com", "", "assistant"}},
		{name: "nested namespace", in: "urn:air:acme.com:finance:tax:agent", want: Identifier{"acme.com", "finance:tax", "agent"}},
		{name: "underscore and dot in name", in: "urn:air:acme.com:ns:tax_agent.v2", want: Identifier{"acme.com", "ns", "tax_agent.v2"}},
		{name: "subdomain publisher", in: "urn:air:alice.github.io:tools:x", want: Identifier{"alice.github.io", "tools", "x"}},
		{name: "reserved placeholder", in: "urn:air:agent.localhost:testing:analyzer", want: Identifier{"agent.localhost", "testing", "analyzer"}},
		{name: "mixed case publisher", in: "urn:air:Acme.COM:ns:x", want: Identifier{"Acme.COM", "ns", "x"}},
		{name: "old ai NID (ADR-0009)", in: "urn:ai:acme.com:ns:x", wantErr: "does not start with"},
		{name: "no name", in: "urn:air:acme.com", wantErr: "no name"},
		{name: "localhost publisher", in: "urn:air:localhost:agent:assistant", wantErr: "not a fully qualified"},
		{name: "IP address publisher", in: "urn:air:10.0.0.1:ns:x", wantErr: "IP address"},
		{name: "underscore in publisher", in: "urn:air:my_host.com:ns:x", wantErr: "invalid DNS label"},
		{name: "leading hyphen label", in: "urn:air:-acme.com:ns:x", wantErr: "invalid DNS label"},
		{name: "trailing dot", in: "urn:air:acme.com.:ns:x", wantErr: "invalid DNS label"},
		{name: "empty label", in: "urn:air:acme..com:ns:x", wantErr: "invalid DNS label"},
		{name: "label over 63", in: "urn:air:" + strings.Repeat("a", 64) + ".com:ns:x", wantErr: "invalid DNS label"},
		{name: "empty namespace segment", in: "urn:air:acme.com::x", wantErr: "namespace segment"},
		{name: "slash in name", in: "urn:air:acme.com:ns:a/b", wantErr: "name"},
		{name: "space in name", in: "urn:air:acme.com:ns:a b", wantErr: "name"},
		{name: "empty name", in: "urn:air:acme.com:ns:", wantErr: "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := ParseIdentifier(tt.in)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.in, got.String())
		})
	}
}

func TestNewIdentifier(t *testing.T) {
	tests := []struct {
		name                       string
		publisher, namespace, item string
		want                       string
		wantErr                    bool
	}{
		{name: "valid", publisher: "acme.com", namespace: "skills", item: "deploy", want: "urn:air:acme.com:skills:deploy"},
		{name: "nested namespace", publisher: "acme.com", namespace: "team:skills", item: "deploy", want: "urn:air:acme.com:team:skills:deploy"},
		{name: "empty namespace", publisher: "acme.com", item: "deploy", wantErr: true},
		{name: "empty publisher", namespace: "ns", item: "deploy", wantErr: true},
		{name: "publisher too long", publisher: strings.Repeat("a.", 127) + "com", namespace: "ns", item: "x", wantErr: true},
		{name: "bad name", publisher: "acme.com", namespace: "ns", item: "a:b", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := NewIdentifier(tt.publisher, tt.namespace, tt.item)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}
