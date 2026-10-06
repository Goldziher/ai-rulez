package okf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRewriteLinks(t *testing.T) {
	upper := func(dest string, _ int) (string, bool) {
		p, suffix, ok := SplitDest(dest)
		if !ok {
			return "", false
		}
		return JoinDest("X/"+p, suffix), true
	}
	tests := []struct {
		name, in, want string
	}{
		{"inline link", "see [a](a.md) now\n", "see [a](X/a.md) now\n"},
		{"fragment and query kept", "[a](a.md#sec) [b](b.md?x=1)", "[a](X/a.md#sec) [b](X/b.md?x=1)"},
		{"title kept", `[a](a.md "the title")`, `[a](X/a.md "the title")`},
		{"image", "![img](pic.png)", "![img](X/pic.png)"},
		{"external and anchor untouched", "[w](https://x.dev/a.md) [m](mailto:a@b.c) [h](#top)", "[w](https://x.dev/a.md) [m](mailto:a@b.c) [h](#top)"},
		{"fenced code untouched", "```\n[a](a.md)\n```\n[b](b.md)\n", "```\n[a](a.md)\n```\n[b](X/b.md)\n"},
		{"tilde fence untouched", "~~~md\n[a](a.md)\n~~~\n", "~~~md\n[a](a.md)\n~~~\n"},
		{"inline code untouched", "use `[a](a.md)` but [b](b.md)", "use `[a](a.md)` but [b](X/b.md)"},
		{"code in link text", "[`a`](a.md)", "[`a`](X/a.md)"},
		{"escaped path", "[a](my%20file.md)", "[a](X/my%20file.md)"},
		{"crlf kept", "[a](a.md)\r\n[b](b.md)\r\n", "[a](X/a.md)\r\n[b](X/b.md)\r\n"},
		{"no links", "plain\ntext", "plain\ntext"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RewriteLinks(tt.in, upper))
		})
	}
}

func TestRewriteLinksReportsLine(t *testing.T) {
	var lines []int
	RewriteLinks("a\n[x](x.md)\n```\n[y](y.md)\n```\n[z](z.md)\n", func(_ string, line int) (string, bool) {
		lines = append(lines, line)
		return "", false
	})
	assert.Equal(t, []int{2, 6}, lines)
}
