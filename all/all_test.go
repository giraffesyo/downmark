package all_test

import (
	"testing"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/all"
)

func TestCanConvert(t *testing.T) {
	cases := []struct {
		name  string
		hints downmark.StreamInfo
		want  bool
	}{
		{"pdf by mime", downmark.StreamInfo{MIMEType: "application/pdf"}, true},
		{"pdf by extension", downmark.StreamInfo{Filename: "doc.pdf"}, true},
		{"docx by extension", downmark.StreamInfo{Filename: "report.docx"}, true},
		{"xlsx by mime", downmark.StreamInfo{MIMEType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}, true},
		{"pptx by extension", downmark.StreamInfo{Filename: "deck.pptx"}, true},
		{"legacy doc by mime", downmark.StreamInfo{MIMEType: "application/msword"}, true},
		{"csv by mime", downmark.StreamInfo{MIMEType: "text/csv"}, true},
		{"html by extension", downmark.StreamInfo{Filename: "page.html"}, true},
		{"plain text is handled elsewhere", downmark.StreamInfo{Filename: "notes.txt"}, false},
		{"source code is handled elsewhere", downmark.StreamInfo{Filename: "main.go"}, false},
		{"unknown binary", downmark.StreamInfo{Filename: "blob.bin", MIMEType: "application/octet-stream"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := all.CanConvert(tc.hints); got != tc.want {
				t.Errorf("CanConvert(%+v) = %v, want %v", tc.hints, got, tc.want)
			}
		})
	}
}
