package lint

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// defaultMaxTableAgeDays is the age `task harness:verify` allows when neither
// HARNESS_MAX_AGE_DAYS nor the repository's [lint.traps] max_table_age_days is set.
const defaultMaxTableAgeDays = 90

func parsePositive(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a positive integer", s)
	}
	return n, nil
}

// repoMaxTableAgeDays reads [lint.traps] max_table_age_days from the
// repository's own config, 0 when unset.
func repoMaxTableAgeDays() int {
	data, err := os.ReadFile("../../.ai-rulez/config.toml")
	if err != nil {
		return 0
	}
	var c struct {
		Lint struct {
			Traps struct {
				Max int `toml:"max_table_age_days"`
			} `toml:"traps"`
		} `toml:"lint"`
	}
	if toml.Unmarshal(data, &c) != nil {
		return 0
	}
	return c.Lint.Traps.Max
}

var (
	tagRe   = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?s)<[^>]*>`) })
	spaceRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\s+`) })
)

// inlineTagRe matches elements that format a run of text without separating
// words, so a page that wraps part of a word in one still contains the quote.
var inlineTagRe = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`(?is)</?(code|a|span|em|strong|b|i|u|kbd|mark|small|sub|sup)(\s[^>]*)?>`)
})

func normaliseQuote(s string) string {
	s = inlineTagRe().ReplaceAllString(s, "")
	s = tagRe().ReplaceAllString(s, " ")
	s = strings.NewReplacer("`", "", "*", "", "’", "'", "&#x27;", "'", "&quot;", `"`, "&amp;", "&", " ", " ").Replace(s)
	return strings.ToLower(spaceRe().ReplaceAllString(s, " "))
}

type quoteRow struct{ id, source, quote string }

// verifyQuotesOnline fetches each distinct source once and reports a quote that
// is no longer on its page. A network failure is a skip of that row, not a pass.
func verifyQuotesOnline(t *testing.T) {
	t.Helper()
	var rows []quoteRow
	traps, _ := Traps() //nolint:errcheck // the table test reports it
	for _, tr := range traps {
		rows = append(rows, quoteRow{tr.Code + " " + tr.Harness, tr.Source, tr.Quote})
	}
	pages := map[string]string{}
	client := &http.Client{Timeout: 30 * time.Second}
	for _, row := range rows {
		page, seen := pages[row.source]
		if !seen {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, row.source, nil) //nolint:errcheck // the URL is a table constant
			resp, err := client.Do(req)
			if err != nil {
				t.Logf("%s: fetch failed: %v", row.id, err)
				pages[row.source] = ""
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) //nolint:errcheck // a short read fails the quote check below
			_ = resp.Body.Close()
			page = normaliseQuote(string(body))
			pages[row.source] = page
		}
		if page == "" {
			continue
		}
		if !strings.Contains(page, normaliseQuote(row.quote)) {
			t.Errorf("%s: quote no longer on %s: %q", row.id, row.source, row.quote)
		}
	}
}

func TestNormaliseQuote(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"block tags and whitespace", "<p>Use  <code>`A`</code>\nB</p>", "use a b"},
		// Cursor's docs wrap the @ in a code element mid-word: "@-mention".
		{"inline markup does not split a word", `when you <code class="x">@</code>-mention the rule`, "when you @-mention the rule"},
		{"emphasis inside a word", "re<em>load</em>ed and <a href=\"/x\">linked</a>", "reloaded and linked"},
		{"block tags still separate words", "<td>one</td><td>two</td>", "one two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.TrimSpace(normaliseQuote(tt.in)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
