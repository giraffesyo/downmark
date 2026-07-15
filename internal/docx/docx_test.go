package docx

import (
	"archive/zip"
	"bytes"
	"maps"
	"strings"
	"testing"
)

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

func buildDocx(t testing.TB, parts map[string]string) ([]byte, int64) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), int64(buf.Len())
}

func docWith(body string, extra map[string]string) map[string]string {
	parts := map[string]string{
		"word/document.xml": `<w:document ` + wNS + `><w:body>` + body + `</w:body></w:document>`,
	}
	maps.Copy(parts, extra)
	return parts
}

func convertBody(t *testing.T, body string, extra map[string]string) string {
	t.Helper()
	data, size := buildDocx(t, docWith(body, extra))
	html, _, err := Convert(t.Context(), bytes.NewReader(data), size, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return html
}

const stylesXML = `<w:styles ` + wNS + `>
<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/></w:style>
<w:style w:type="paragraph" w:styleId="Titre3"><w:name w:val="titre 3"/><w:basedOn w:val="Heading3"/></w:style>
<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/></w:style>
<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/></w:style>
<w:style w:type="paragraph" w:styleId="Fancy"><w:name w:val="Fancy"/><w:pPr><w:outlineLvl w:val="3"/></w:pPr></w:style>
</w:styles>`

func p(style, text string) string {
	pr := ""
	if style != "" {
		pr = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + pr + `<w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func TestHeadingsAndStyles(t *testing.T) {
	body := p("Heading1", "H One") + p("Heading2", "H Two") +
		p("Titre3", "Based On") + p("Fancy", "Outline") + p("", "plain")
	html := convertBody(t, body, map[string]string{"word/styles.xml": stylesXML})
	for _, want := range []string{
		"<h1>H One</h1>", "<h2>H Two</h2>", "<h3>Based On</h3>",
		"<h4>Outline</h4>", "<p>plain</p>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

func TestRunFormatting(t *testing.T) {
	body := `<w:p><w:r><w:rPr><w:b/><w:i/></w:rPr><w:t>both</w:t></w:r>
<w:r><w:rPr><w:b w:val="0"/></w:rPr><w:t>notbold</w:t></w:r>
<w:r><w:rPr><w:strike/></w:rPr><w:t>gone</w:t></w:r>
<w:r><w:rPr><w:vertAlign w:val="superscript"/></w:rPr><w:t>2</w:t></w:r></w:p>`
	html := convertBody(t, body, nil)
	for _, want := range []string{
		"<strong><em>both</em></strong>", ">notbold<", "<del>gone</del>", "<sup>2</sup>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<strong>notbold") {
		t.Errorf("explicit w:val=0 must disable bold:\n%s", html)
	}
}

const numberingXML = `<w:numbering ` + wNS + `>
<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl><w:lvl w:ilvl="1"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>
<w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum>
<w:num w:numId="10"><w:abstractNumId w:val="0"/></w:num>
<w:num w:numId="11"><w:abstractNumId w:val="1"/></w:num>
</w:numbering>`

func li(numID, ilvl, text string) string {
	return `<w:p><w:pPr><w:numPr><w:ilvl w:val="` + ilvl + `"/><w:numId w:val="` + numID + `"/></w:numPr></w:pPr>` +
		`<w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func TestLists(t *testing.T) {
	body := li("10", "0", "alpha") + li("10", "1", "nested") + li("10", "0", "beta") +
		li("11", "0", "first") + li("11", "0", "second") + p("", "after")
	html := convertBody(t, body, map[string]string{"word/numbering.xml": numberingXML})
	squeezed := strings.Join(strings.Fields(html), "")
	for _, want := range []string{
		"<ul><li>alpha<ul><li>nested</li></ul></li><li>beta</li></ul>",
		"<ol><li>first</li><li>second</li></ol>",
	} {
		if !strings.Contains(squeezed, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

func TestHyperlinkAndAnchor(t *testing.T) {
	rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/x" TargetMode="External"/>
</Relationships>`
	body := `<w:p><w:hyperlink r:id="rId5"><w:r><w:t>ext</w:t></w:r></w:hyperlink>
<w:hyperlink w:anchor="sec1"><w:r><w:t>internal</w:t></w:r></w:hyperlink></w:p>`
	html := convertBody(t, body, map[string]string{"word/_rels/document.xml.rels": rels})
	if !strings.Contains(html, `<a href="https://example.com/x">ext</a>`) {
		t.Errorf("external link wrong:\n%s", html)
	}
	if !strings.Contains(html, "internal") || strings.Contains(html, `href=""`) {
		t.Errorf("internal anchor should degrade to text:\n%s", html)
	}
}

func TestTableGridSpanAndVMerge(t *testing.T) {
	body := `<w:tbl>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/></w:tcPr><w:p><w:r><w:t>wide</w:t></w:r></w:p></w:tc>
<w:tc><w:p><w:r><w:t>h3</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>a</w:t></w:r></w:p></w:tc>
<w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>merged</w:t></w:r></w:p></w:tc>
<w:tc><w:p><w:r><w:t>c</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>d</w:t></w:r></w:p></w:tc>
<w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc>
<w:tc><w:p><w:r><w:t>f</w:t></w:r></w:p></w:tc></w:tr>
</w:tbl>`
	html := convertBody(t, body, nil)
	squeezed := strings.Join(strings.Fields(html), "")
	if !strings.Contains(squeezed, "<th>wide</th><th></th><th>h3</th>") {
		t.Errorf("gridSpan not expanded:\n%s", html)
	}
	if !strings.Contains(squeezed, "<td>d</td><td></td><td>f</td>") {
		t.Errorf("vMerge continuation should be empty cell:\n%s", html)
	}
}

func TestInsKeptDelDropped(t *testing.T) {
	body := `<w:p><w:ins><w:r><w:t>added</w:t></w:r></w:ins><w:del><w:r><w:t>removed</w:t></w:r></w:del></w:p>`
	html := convertBody(t, body, nil)
	if !strings.Contains(html, "added") || strings.Contains(html, "removed") {
		t.Errorf("track changes handling wrong:\n%s", html)
	}
}

func TestSdtUnwrapped(t *testing.T) {
	body := `<w:sdt><w:sdtContent><w:p><w:r><w:t>inside sdt</w:t></w:r></w:p></w:sdtContent></w:sdt>`
	html := convertBody(t, body, nil)
	if !strings.Contains(html, "inside sdt") {
		t.Errorf("sdt content lost:\n%s", html)
	}
}

func TestMathFallback(t *testing.T) {
	body := `<w:p xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">` +
		`<m:oMath><m:r><m:t>E=mc2</m:t></m:r></m:oMath></w:p>`
	html := convertBody(t, body, nil)
	if !strings.Contains(html, "E=mc2") {
		t.Errorf("math text lost:\n%s", html)
	}
}

func TestEscaping(t *testing.T) {
	body := `<w:p><w:r><w:t>1 &lt; 2 &amp; &quot;q&quot;</w:t></w:r></w:p>`
	html := convertBody(t, body, nil)
	if !strings.Contains(html, "1 &lt; 2 &amp;") {
		t.Errorf("text not escaped properly:\n%s", html)
	}
}

func TestTitleExtraction(t *testing.T) {
	body := p("Title", "My Document") + p("", "body text")
	data, size := buildDocx(t, docWith(body, map[string]string{"word/styles.xml": stylesXML}))
	_, title, err := Convert(t.Context(), bytes.NewReader(data), size, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if title != "My Document" {
		t.Errorf("title = %q", title)
	}
}

func FuzzConvert(f *testing.F) {
	seed, _ := buildDocx(f, map[string]string{
		"word/document.xml": `<w:document ` + wNS + `><w:body><w:p><w:r><w:t>seed</w:t></w:r></w:p></w:body></w:document>`,
	})
	f.Add(seed)
	f.Add([]byte("PK\x03\x04 garbage"))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic; errors are fine.
		_, _, _ = Convert(t.Context(), bytes.NewReader(data), int64(len(data)), Options{})
	})
}
