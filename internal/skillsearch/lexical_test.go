package skillsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want []string
	}{
		{"Run the Migrations", []string{"run", "migration"}},
		{"refunding customers", []string{"refund", "customer"}},
		{"policies and queries", []string{"policy", "query"}},
		{"the of and", []string{}},
		{"CamelCase-and_snake 42", []string{"camelcas", "snak", "42"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, append([]string{}, Tokenize(tt.in)...), tt.in)
	}
}

func TestRank(t *testing.T) {
	t.Parallel()
	docs := []Doc{
		{Name: "gamma", Description: "Mentions deploy once in the description"},
		{Name: "beta", Description: "Something else", Triggers: []string{"deploy to production"}},
		{Name: "alpha", Description: "Mentions deploy once in the description"},
	}
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"trigger beats description, ties by name", "deploy", []string{"beta", "alpha", "gamma"}},
		{"stemmed query", "deploying", []string{"beta", "alpha", "gamma"}},
		{"stopword-only query", "the of and", nil},
		{"empty query", "", nil},
		{"no match", "zebra", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			hits := Rank(docs, tt.query)

			// Assert
			var got []string
			for _, h := range hits {
				got = append(got, docs[h.Index].Name)
			}
			assert.Equal(t, tt.want, got)
		})
	}
	assert.Nil(t, Rank(nil, "deploy"))
}

func TestRank_Deterministic(t *testing.T) {
	t.Parallel()
	docs := []Doc{{Name: "a", Description: "deploy"}, {Name: "b", Description: "deploy"}, {Name: "c", Keywords: []string{"deploy"}}}
	first := Rank(docs, "deploy")
	require.Len(t, first, 3)
	for range 20 {
		assert.Equal(t, first, Rank(docs, "deploy"))
	}
}

func TestStemConflatesInflectionsOfOneWord(t *testing.T) {
	t.Parallel()
	groups := [][]string{
		{"image", "images"},
		{"template", "templates"},
		{"service", "services"},
		{"database", "databases"},
		{"release", "releases", "releasing", "released"},
		{"cache", "caches", "cached", "caching"},
		{"migration", "migrations"},
		{"match", "matches"},
		{"process", "processes", "processing"},
		{"policy", "policies"},
	}
	for _, group := range groups {
		want := stem(group[0])
		for _, w := range group[1:] {
			assert.Equal(t, want, stem(w), "%s should stem like %s", w, group[0])
		}
	}
}

func TestStemKeepsDistinctWordsApart(t *testing.T) {
	t.Parallel()
	assert.NotEqual(t, stem("class"), stem("clas"))
	assert.Equal(t, "class", stem("class"), "a double s is not a plural")
	assert.NotEqual(t, stem("image"), stem("migration"))
	assert.Equal(t, "use", stem("use"), "short words are left alone")
}
