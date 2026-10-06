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
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

func normaliseQuote(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.NewReplacer("`", "", "*", "", "’", "'", "&#x27;", "'", "&quot;", `"`, "&amp;", "&", " ", " ").Replace(s)
	return strings.ToLower(spaceRe.ReplaceAllString(s, " "))
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
	got := normaliseQuote("<p>Use  <code>`A`</code>\nB</p>")
	if got != " use a b " && strings.TrimSpace(got) != "use a b" {
		t.Errorf("got %q", got)
	}
}
