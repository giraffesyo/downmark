package downmark_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giraffesyo/pdf/pdftest"
)

// TestWriteSyntheticFixture regenerates testdata/synthetic.pdf when run
// with -update. The fixture reproduces real-world PDF structure (all
// content inside a Form XObject as in Google Docs exports, a composite
// Identity-H font decoded via ToUnicode, TJ-array word gaps) with
// entirely synthetic content.
func TestWriteSyntheticFixture(t *testing.T) {
	if !*updateGolden {
		t.Skip("run with -update to regenerate the fixture")
	}
	path := filepath.Join("testdata", "synthetic.pdf")
	if err := os.WriteFile(path, buildSyntheticPDF(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func buildSyntheticPDF() []byte {
	const headline = "SYNTHETIC HEADLINE 123"

	// Composite-font machinery for the headline: code i+1 → rune i,
	// decoded purely through the ToUnicode CMap.
	var bfchars, codes strings.Builder
	runes := []rune(headline)
	for i, r := range runes {
		fmt.Fprintf(&bfchars, "<%04X> <%04X>\n", i+1, r)
	}
	codes.WriteByte('<')
	for i := range runes {
		fmt.Fprintf(&codes, "%04X", i+1)
	}
	codes.WriteByte('>')
	cmap := pdftest.ToUnicodeCMap(fmt.Sprintf("%d beginbfchar\n%sendbfchar", len(runes), bfchars.String()))

	form := pdftest.Stream(
		"/Type /XObject /Subtype /Form /BBox [0 0 612 792] "+
			"/Resources << /Font << /F1 5 0 R /F2 7 0 R >> >>",
		`BT /F2 16 Tf 72 720 Td `+codes.String()+` Tj ET
BT /F1 10 Tf 14 TL 72 695 Td
(part one \(alpha\) | part two \(beta\) | part three) Tj
0 -30 Td
(SECTION ONE) Tj
T*
(item line - example text \(one - two\)) Tj
T*
[(kerned segments need) -600 (a space here.)] TJ
0 -30 Td
(SECTION TWO) Tj
T*
(closing line of the synthetic document \(three - four\)) Tj
ET`)

	return pdftest.Build(1,
		pdftest.Catalog(2),
		pdftest.Pages(3),
		pdftest.Page(2, 4, "<< /XObject << /X1 6 0 R >> >>"),
		pdftest.Stream("", "q /X1 Do Q"),
		pdftest.Helvetica(),
		form,
		pdftest.Type0Font(8, 9),
		pdftest.CIDFont(fmt.Sprintf("/W [1 %d 700]", len(runes))),
		cmap,
	)
}
