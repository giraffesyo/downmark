package downmark_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/all"
)

// vector describes one conversion test case over a testdata file, in the
// style of upstream markitdown's test vectors: substring assertions that are
// robust to formatting drift.
type vector struct {
	file           string
	hints          downmark.StreamInfo
	mustInclude    []string
	mustNotInclude []string
	wantTitle      string // checked only if non-empty
}

var vectors = []vector{
	{
		file:  "test.pdf",
		hints: downmark.StreamInfo{MIMEType: "application/pdf"},
		mustInclude: []string{
			"While there is contemporaneous exploration of multi-agent approaches",
		},
	},
	{
		// Synthetic document reproducing real-world PDF structure: all
		// text inside a Form XObject (Google Docs export pattern), headline
		// in an Identity-H composite font decoded via ToUnicode, word gaps
		// from TJ kerning arrays.
		file:  "synthetic.pdf",
		hints: downmark.StreamInfo{MIMEType: "application/pdf"},
		mustInclude: []string{
			"SYNTHETIC HEADLINE 123",
			"part one (alpha) | part two (beta) | part three",
			"SECTION ONE",
			"item line - example text (one - two)",
			"kerned segments need a space here.",
			"closing line of the synthetic document (three - four)",
		},
		mustNotInclude: []string{"�"},
	},
	{
		file:  "test_blog.html",
		hints: downmark.StreamInfo{MIMEType: "text/html", Charset: "utf-8"},
		mustInclude: []string{
			"Large language models (LLMs) are powerful tools that can generate natural language texts for various applications, such as chatbots, summarization, translation, and more. GPT-4 is currently the state of the art LLM in the world. Is model selection irrelevant? What about inference parameters?",
			"an example where high cost can easily prevent a generic complex",
		},
		mustNotInclude: []string{"<script", "javascript:"},
	},
	{
		file:  "test_wikipedia.html",
		hints: downmark.StreamInfo{MIMEType: "text/html", Charset: "utf-8"},
		mustInclude: []string{
			"Microsoft entered the operating system (OS) business in 1980 with its own version of ",
			"Bill Gates",
		},
		mustNotInclude: []string{"<script"},
	},
	{
		file:  "test.xlsx",
		hints: downmark.StreamInfo{MIMEType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		mustInclude: []string{
			"## 09060124-b5e7-4717-9d07-3c046eb",
			"6ff4173b-42a5-4784-9b19-f49caff4d93d",
			"affc7dad-52dc-4b98-9b5d-51e65d8a8ad0",
		},
	},
	{
		file:  "test.docx",
		hints: downmark.StreamInfo{MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		mustInclude: []string{
			"314b0a30-5b04-470b-b9f7-eed2c2bec74a",
			"49e168b7-d2ae-407f-a055-2167576f39a1",
			"## d666f1f7-46cb-42bd-9a39-9a39cf2a509f",
			"# Abstract",
			"# Introduction",
			"AutoGen: Enabling Next-Gen LLM Applications via Multi-Agent Conversation",
		},
		mustNotInclude: []string{
			"data:image/png;base64,iVBORw0KGgoAAAANSU",
		},
	},
	{
		file:  "test.pptx",
		hints: downmark.StreamInfo{MIMEType: "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		mustInclude: []string{
			"2cdda5c8-e50e-4db4-b5f0-9722a649f455",
			"04191ea8-5c73-4215-a1d3-1cfb43aaaf12",
			"44bf7d06-5e7a-4a40-a2e1-a2e42ef28c8a",
			"1b92870d-e3b5-4e65-8153-919f4ff45592",
			"# AutoGen: Enabling Next-Gen LLM Applications via Multi-Agent Conversation",
			"a3f6004b-6f4f-4ea8-bee3-3741f4dc385f", // chart title
			"2003",                                 // chart category
			"![This phrase of the caption is Human-written.](image2.jpg)",
			"<!-- Slide number: 1 -->",
			"<!-- Slide number: 6 -->",
		},
		mustNotInclude: []string{"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQE"},
	},
	{
		file:  "test_mskanji.csv",
		hints: downmark.StreamInfo{MIMEType: "text/csv", Charset: "cp932"},
		mustInclude: []string{
			"| 名前 | 年齢 | 住所 |",
			"| --- | --- | --- |",
			"| 佐藤太郎 | 30 | 東京 |",
			"| 三木英子 | 25 | 大阪 |",
			"| 髙橋淳 | 35 | 名古屋 |",
		},
	},
}

func checkVector(t *testing.T, v vector, res *downmark.Result, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("convert %s: %v", v.file, err)
	}
	for _, want := range v.mustInclude {
		if !strings.Contains(res.Markdown, want) {
			t.Errorf("%s: output missing %q\n---- output head ----\n%s",
				v.file, want, head(res.Markdown, 2000))
		}
	}
	for _, bad := range v.mustNotInclude {
		if strings.Contains(res.Markdown, bad) {
			t.Errorf("%s: output must not contain %q", v.file, bad)
		}
	}
	if v.wantTitle != "" && res.Title != v.wantTitle {
		t.Errorf("%s: title = %q, want %q", v.file, res.Title, v.wantTitle)
	}
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[...truncated...]"
}

// TestVectorsConvertFile converts each vector by path, hints derived from
// the filename only.
func TestVectorsConvertFile(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.file, func(t *testing.T) {
			res, err := all.ConvertFile(context.Background(), filepath.Join("testdata", v.file))
			checkVector(t, v, res, err)
		})
	}
}

// TestVectorsWithHints converts each vector as a stream with full hints.
func TestVectorsWithHints(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.file, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", v.file))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }() // read-only handle
			res, err := all.Convert(context.Background(), f, v.hints)
			checkVector(t, v, res, err)
		})
	}
}

// TestVectorsNoHints converts each vector as a bare stream: detection has to
// figure everything out from content alone.
func TestVectorsNoHints(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", v.file))
			if err != nil {
				t.Fatal(err)
			}
			res, err := all.Convert(context.Background(), strings.NewReader(string(data)), downmark.StreamInfo{})
			checkVector(t, v, res, err)
		})
	}
}
