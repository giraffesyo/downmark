package ocr

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// tsvWordLevel is the value tesseract's level column carries for a row
// describing a word; the smaller numbers describe the page, blocks,
// paragraphs and lines that contain it.
const tsvWordLevel = 5

// maxTSVLine bounds one row of TSV. A row holds a single word, so the
// bound is generous, and it keeps a file that is not TSV at all from
// being buffered whole.
const maxTSVLine = 1 << 20

// ParseTesseractTSV reads the TSV that `tesseract <image> - tsv` writes
// and returns its words, in the order tesseract reported them.
//
// Columns are located by the header's names rather than by position, so
// the shape of the row can change between tesseract versions without
// breaking this. Rows that describe a block, paragraph or line rather
// than a word are skipped, as are words with no text; Word.Conf carries
// the confidence for a caller that wants to filter on it.
//
// The same TSV is what tesseract's `hocr` and `alto` outputs describe in
// XML; this reads only TSV, which is the cheapest of the three to parse
// and carries everything the seam needs.
func ParseTesseractTSV(r io.Reader) ([]Word, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxTSVLine)

	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("ocr: reading tesseract TSV: %w", err)
		}
		return nil, errors.New("ocr: tesseract produced no output")
	}
	cols := make(map[string]int)
	for i, name := range strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t") {
		cols[strings.TrimSpace(name)] = i
	}
	for _, required := range []string{"left", "top", "width", "height", "text"} {
		if _, ok := cols[required]; !ok {
			return nil, fmt.Errorf("ocr: tesseract TSV has no %q column; is this TSV output?", required)
		}
	}

	var words []Word
	for line := 2; sc.Scan(); line++ {
		fields := strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t")
		field := func(name string) string {
			i, ok := cols[name]
			if !ok || i >= len(fields) {
				return ""
			}
			return fields[i]
		}
		// A row that names a containing block repeats its children's
		// text, so taking every row would duplicate the whole page.
		if level := field("level"); level != "" && level != strconv.Itoa(tsvWordLevel) {
			continue
		}
		text := strings.TrimSpace(field("text"))
		if text == "" {
			continue
		}
		w := Word{Text: text}
		var err error
		for _, box := range []struct {
			name string
			dst  *float64
		}{
			{"left", &w.Left}, {"top", &w.Top},
			{"width", &w.Width}, {"height", &w.Height},
		} {
			if *box.dst, err = strconv.ParseFloat(field(box.name), 64); err != nil {
				return nil, fmt.Errorf("ocr: tesseract TSV line %d: %s: %w", line, box.name, err)
			}
		}
		// Confidence is advisory, so a version that omits it or writes
		// something unparseable leaves the zero value rather than
		// failing the page.
		w.Conf, _ = strconv.ParseFloat(field("conf"), 64)
		words = append(words, w)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("ocr: reading tesseract TSV: %w", err)
	}
	return words, nil
}
