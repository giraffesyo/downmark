package htmlmd

import (
	"strings"
	"testing"
)

func conv(t *testing.T, in string, opts Options) (string, string) {
	t.Helper()
	md, title, err := ConvertString(t.Context(), in, opts)
	if err != nil {
		t.Fatalf("ConvertString: %v", err)
	}
	return md, title
}

func TestBasics(t *testing.T) {
	md, title := conv(t, `<html><head><title> My  Page </title></head>
<body><h1>Head</h1><p>Some <b>bold</b> and <i>italic</i> text.</p></body></html>`, Options{})
	if title != "My Page" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"# Head", "**bold**", "*italic*"} {
		if !strings.Contains(md, want) {
			t.Errorf("output missing %q in %q", want, md)
		}
	}
}

func TestScriptAndStyleStripped(t *testing.T) {
	md, _ := conv(t, `<body><script>alert(1)</script><style>p{}</style><p>keep</p></body>`, Options{})
	if strings.Contains(md, "alert") || strings.Contains(md, "p{}") {
		t.Errorf("script/style leaked into %q", md)
	}
	if !strings.Contains(md, "keep") {
		t.Errorf("content lost: %q", md)
	}
}

func TestJavascriptLinkUnwrapped(t *testing.T) {
	md, _ := conv(t, `<body><a href="javascript:evil()">click me</a> <a href="https://example.com">ok</a></body>`, Options{})
	if strings.Contains(md, "javascript:") {
		t.Errorf("javascript: link survived: %q", md)
	}
	if !strings.Contains(md, "click me") {
		t.Errorf("link text lost: %q", md)
	}
	if !strings.Contains(md, "https://example.com") {
		t.Errorf("safe link lost: %q", md)
	}
}

func TestDataURITruncated(t *testing.T) {
	in := `<body><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==" alt="pic"></body>`
	md, _ := conv(t, in, Options{})
	if strings.Contains(md, "iVBORw0") {
		t.Errorf("data URI not truncated: %q", md)
	}
	if !strings.Contains(md, "data:image/png;base64,...") {
		t.Errorf("expected truncated data URI marker in %q", md)
	}
	md, _ = conv(t, in, Options{KeepDataURIs: true})
	if !strings.Contains(md, "iVBORw0KGgoAAAANSUhEUg==") {
		t.Errorf("data URI should be kept: %q", md)
	}
}

func TestTable(t *testing.T) {
	md, _ := conv(t, `<body><table>
<tr><th>Name</th><th>Age</th></tr>
<tr><td>Alice</td><td>30</td></tr>
<tr><td>Bob | Bobby</td><td>25</td></tr>
</table></body>`, Options{})
	// The table plugin pads cells for alignment; compare space-insensitively.
	squeezed := strings.Join(strings.Fields(md), " ")
	for _, want := range []string{"| Name | Age |", "| Alice | 30 |"} {
		if !strings.Contains(squeezed, want) {
			t.Errorf("output missing %q:\n%s", want, md)
		}
	}
	// The pipe inside a cell must not break the row structure.
	for line := range strings.SplitSeq(md, "\n") {
		if strings.Contains(line, "Bobby") && strings.Count(line, "|")-strings.Count(line, `\|`) != 3 {
			t.Errorf("pipe not escaped in table row: %q", line)
		}
	}
}

func TestLists(t *testing.T) {
	md, _ := conv(t, `<body><ul><li>one</li><li>two</li></ul><ol><li>first</li><li>second</li></ol></body>`, Options{})
	for _, want := range []string{"- one", "- two", "1. first", "2. second"} {
		if !strings.Contains(md, want) {
			t.Errorf("output missing %q:\n%s", want, md)
		}
	}
}

func TestChildOfRemovedNodeNotConverted(t *testing.T) {
	md, _ := conv(t, `<body><noscript><p>fallback junk</p></noscript><p>real</p></body>`, Options{})
	if strings.Contains(md, "fallback junk") {
		t.Errorf("noscript content leaked: %q", md)
	}
}

func TestStrikethrough(t *testing.T) {
	md, _ := conv(t, `<body><p><del>gone</del></p></body>`, Options{})
	if !strings.Contains(md, "~~gone~~") {
		t.Errorf("strikethrough missing: %q", md)
	}
}
