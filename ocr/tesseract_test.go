package ocr_test

import (
	"strings"
	"testing"

	"github.com/giraffesyo/downmark/ocr"
)

// tesseractTSV is the shape `tesseract page.png - tsv` writes: a header
// naming the columns, rows for the page, block, paragraph and line that
// contain each word, and then the words themselves at level 5.
const tesseractTSV = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"1\t1\t0\t0\t0\t0\t0\t0\t1240\t1750\t-1\t\n" +
	"2\t1\t1\t0\t0\t0\t100\t200\t900\t60\t-1\t\n" +
	"3\t1\t1\t1\t0\t0\t100\t200\t900\t60\t-1\t\n" +
	"4\t1\t1\t1\t1\t0\t100\t200\t900\t60\t-1\t\n" +
	"5\t1\t1\t1\t1\t1\t100\t200\t180\t40\t96.31\tInvoice\n" +
	"5\t1\t1\t1\t1\t2\t300\t205\t120\t35\t42.07\tNo.\n" +
	"5\t1\t1\t1\t1\t3\t440\t200\t90\t40\t12.5\t\n"

func TestParseTesseractTSVReadsWordsAndSkipsTheRest(t *testing.T) {
	words, err := ocr.ParseTesseractTSV(strings.NewReader(tesseractTSV))
	if err != nil {
		t.Fatalf("ParseTesseractTSV: %v", err)
	}
	// The page, block, paragraph and line rows repeat their children's
	// extent; taking them would duplicate the page. The last word row
	// has no text at all.
	want := []ocr.Word{
		{Text: "Invoice", Left: 100, Top: 200, Width: 180, Height: 40, Conf: 96.31},
		{Text: "No.", Left: 300, Top: 205, Width: 120, Height: 35, Conf: 42.07},
	}
	if len(words) != len(want) {
		t.Fatalf("words = %+v, want %d of them", words, len(want))
	}
	for i, w := range want {
		if words[i] != w {
			t.Errorf("word %d = %+v, want %+v", i, words[i], w)
		}
	}
}

// Columns are located by name so that a tesseract release that adds or
// reorders one does not silently shift every box.
func TestParseTesseractTSVLocatesColumnsByName(t *testing.T) {
	reordered := "text\theight\twidth\ttop\tleft\tlevel\n" +
		"Total\t40\t180\t200\t100\t5\n"
	words, err := ocr.ParseTesseractTSV(strings.NewReader(reordered))
	if err != nil {
		t.Fatalf("ParseTesseractTSV: %v", err)
	}
	want := ocr.Word{Text: "Total", Left: 100, Top: 200, Width: 180, Height: 40}
	if len(words) != 1 || words[0] != want {
		t.Fatalf("words = %+v, want %+v", words, want)
	}
}

// A missing confidence column is survivable — it is advisory — but a
// missing box is not, and quietly placing every word at the origin would
// be worse than failing.
func TestParseTesseractTSVWithoutConfidence(t *testing.T) {
	words, err := ocr.ParseTesseractTSV(strings.NewReader("level\tleft\ttop\twidth\theight\ttext\n5\t1\t2\t3\t4\tword\n"))
	if err != nil {
		t.Fatalf("ParseTesseractTSV: %v", err)
	}
	if len(words) != 1 || words[0].Conf != 0 {
		t.Fatalf("words = %+v, want one word with no confidence", words)
	}
}

func TestParseTesseractTSVRejectsOutputThatIsNotTSV(t *testing.T) {
	for name, input := range map[string]string{
		"plain text": "Invoice No. 12345\nDated 4 March\n",
		"empty":      "",
		"no boxes":   "level\tconf\ttext\n5\t90\tword\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ocr.ParseTesseractTSV(strings.NewReader(input)); err == nil {
				t.Error("err = nil, want a refusal so the caller can fall back to reading it as text")
			}
		})
	}
}

func TestParseTesseractTSVReportsAnUnreadableBox(t *testing.T) {
	input := "level\tleft\ttop\twidth\theight\ttext\n5\tten\t2\t3\t4\tword\n"
	err := func() error {
		_, err := ocr.ParseTesseractTSV(strings.NewReader(input))
		return err
	}()
	if err == nil {
		t.Fatal("err = nil, want the unparseable left column reported")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "left") {
		t.Errorf("err = %v, want it to name the line and column", err)
	}
}

// Windows line endings survive a round trip through a pipe on some
// engines; a stray carriage return must not land in the text or the box.
func TestParseTesseractTSVToleratesCarriageReturns(t *testing.T) {
	input := "level\tleft\ttop\twidth\theight\ttext\r\n5\t1\t2\t3\t4\tword\r\n"
	words, err := ocr.ParseTesseractTSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseTesseractTSV: %v", err)
	}
	want := ocr.Word{Text: "word", Left: 1, Top: 2, Width: 3, Height: 4}
	if len(words) != 1 || words[0] != want {
		t.Fatalf("words = %+v, want %+v", words, want)
	}
}
