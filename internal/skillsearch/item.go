package skillsearch

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// Item is one searchable skill with its identity: the id is the catalog name,
// the digest the skill's lock digest. Body is SKILL.md without its frontmatter;
// it is embedded only when [search] index_body is set.
type Item struct {
	ID     string
	Domain string
	Digest string
	Body   string
	Doc    Doc
}

// ItemKind is the kind recorded in the manifest.
const ItemKind = "skill"

// DocTemplateVersion versions the embedded text template. Bump it when
// EmbedText changes shape: every index then needs a rebuild.
const DocTemplateVersion = 1

// maxQueryEmbedBytes caps the query text sent to an embedder.
const maxQueryEmbedBytes = 2048

// EmbedText is the text of an item that is sent to the embedder: a fixed
// template over the configured fields, so the same skill always embeds the same
// bytes. Empty fields are left out; an item with an empty description embeds its
// name only.
func EmbedText(it *Item, cfg Config) string {
	r := cfg.Resolved()
	var b strings.Builder
	line := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(strings.Join(strings.Fields(value), " "))
		b.WriteByte('\n')
	}
	has := func(f string) bool {
		for _, x := range r.Fields {
			if x == f {
				return true
			}
		}
		return false
	}
	if has(FieldName) {
		line("name", it.Doc.Name)
	}
	if has(FieldDescription) {
		line("description", it.Doc.Description)
	}
	if has(FieldTriggers) {
		line("triggers", strings.Join(it.Doc.Triggers, "; "))
	}
	if has(FieldKeywords) {
		line("keywords", strings.Join(it.Doc.Keywords, ", "))
	}
	if r.IndexBody {
		line("body", truncateRunes(it.Body, r.BodyChars))
	}
	return strings.TrimRight(b.String(), "\n")
}

// TextDigest is the digest of an embedded text; it keys vector reuse.
func TextDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func truncateRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func capQuery(q string) string {
	if len(q) <= maxQueryEmbedBytes {
		return q
	}
	q = q[:maxQueryEmbedBytes]
	for q != "" && !utf8.ValidString(q) {
		q = q[:len(q)-1]
	}
	return q
}

// Docs returns the lexical documents of items, in order.
func Docs(items []Item) []Doc {
	docs := make([]Doc, len(items))
	for i := range items {
		docs[i] = items[i].Doc
	}
	return docs
}

// IDs returns the ids of items, in order.
func IDs(items []Item) []string {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	return ids
}
