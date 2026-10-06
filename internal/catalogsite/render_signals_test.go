package catalogsite

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

func TestRender_EvalAndUsageSectionsAppearOnlyWhenPresent(t *testing.T) {
	// Arrange
	delta := 0.25
	doc := hostileDoc()
	doc.Items = doc.Items[:2]
	doc.Items[0].Eval = &govview.ItemEval{Cases: 4, PassRate: 0.75, Passing: true, AblationDelta: &delta, Stale: true, Verified: true, Date: hostile[0]}
	doc.Items[0].Usage = &govview.ItemUsage{Invocations: 14, LastSeen: hostile[1]}

	// Act
	site, err := Render(doc, Options{})
	require.NoError(t, err)
	doc.Items[0].Eval, doc.Items[0].Usage = nil, nil
	bare, bareErr := Render(doc, Options{})

	// Assert
	require.NoError(t, bareErr)
	index := string(site.Files["index.html"])
	assert.Contains(t, index, `<th scope="col">Eval</th>`)
	assert.Contains(t, index, `<th scope="col" class="num">Uses</th>`)
	assert.Contains(t, index, "75% (stale)")
	assert.NotContains(t, string(bare.Files["index.html"]), `>Eval<`)
	var item string
	for name, data := range site.Files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		checkHTML(t, name, string(data))
		assert.NotContains(t, string(data), hostile[0], name)
		if strings.HasPrefix(name, "items/") && strings.Contains(string(data), "Recorded eval result") {
			item = string(data)
		}
	}
	assert.Contains(t, item, "75% over 4 scored case(s)")
	assert.Contains(t, item, "25%")
	assert.Contains(t, item, "Invocations")
}
