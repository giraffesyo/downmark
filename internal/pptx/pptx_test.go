package pptx

import (
	"archive/zip"
	"bytes"
	"maps"
	"strings"
	"testing"
)

// buildPptx assembles an in-memory .pptx archive from part name → content.
func buildPptx(t testing.TB, parts map[string]string) ([]byte, int64) {
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

const nsDecls = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`

func minimalDeck(slide1 string, extra map[string]string) map[string]string {
	parts := map[string]string{
		"ppt/presentation.xml": `<p:presentation ` + nsDecls + `><p:sldIdLst>` +
			`<p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml": slide1,
	}
	maps.Copy(parts, extra)
	return parts
}

func convertDeck(t *testing.T, parts map[string]string) string {
	t.Helper()
	data, size := buildPptx(t, parts)
	md, _, err := Convert(t.Context(), bytes.NewReader(data), size, 0)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return md
}

func TestTitleAndBodyOrdering(t *testing.T) {
	slide := `<p:sld ` + nsDecls + `><p:cSld><p:spTree>
<p:sp><p:nvSpPr><p:cNvPr id="3" name="Body"/><p:nvPr/></p:nvSpPr>
  <p:spPr><a:xfrm><a:off x="100" y="2000"/></a:xfrm></p:spPr>
  <p:txBody><a:p><a:r><a:t>body below</a:t></a:r></a:p></p:txBody></p:sp>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr>
  <p:spPr><a:xfrm><a:off x="100" y="100"/></a:xfrm></p:spPr>
  <p:txBody><a:p><a:r><a:t>Deck Title</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld></p:sld>`
	data, size := buildPptx(t, minimalDeck(slide, nil))
	md, title, err := Convert(t.Context(), bytes.NewReader(data), size, 0)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Deck Title" {
		t.Errorf("title = %q", title)
	}
	iTitle := strings.Index(md, "# Deck Title")
	iBody := strings.Index(md, "body below")
	if iTitle < 0 || iBody < 0 || iTitle > iBody {
		t.Errorf("expected title before body:\n%s", md)
	}
}

func TestRunsAndBreaksInterleave(t *testing.T) {
	slide := `<p:sld ` + nsDecls + `><p:cSld><p:spTree>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="x"/><p:nvPr/></p:nvSpPr><p:spPr/>
  <p:txBody><a:p><a:r><a:t>line one</a:t></a:r><a:br/><a:r><a:t>line two</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld></p:sld>`
	md := convertDeck(t, minimalDeck(slide, nil))
	if !strings.Contains(md, "line one\nline two") {
		t.Errorf("br not preserved between runs:\n%q", md)
	}
}

func TestTable(t *testing.T) {
	slide := `<p:sld ` + nsDecls + `><p:cSld><p:spTree>
<p:graphicFrame><p:xfrm><a:off x="1" y="1"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table">
<a:tbl><a:tr><a:tc><a:txBody><a:p><a:r><a:t>H1</a:t></a:r></a:p></a:txBody></a:tc>
<a:tc><a:txBody><a:p><a:r><a:t>H2</a:t></a:r></a:p></a:txBody></a:tc></a:tr>
<a:tr><a:tc><a:txBody><a:p><a:r><a:t>c1</a:t></a:r></a:p></a:txBody></a:tc>
<a:tc><a:txBody><a:p><a:r><a:t>c2</a:t></a:r></a:p></a:txBody></a:tc></a:tr></a:tbl>
</a:graphicData></a:graphic></p:graphicFrame>
</p:spTree></p:cSld></p:sld>`
	md := convertDeck(t, minimalDeck(slide, nil))
	for _, want := range []string{"| H1 | H2 |", "| --- | --- |", "| c1 | c2 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestGroupRecursionAndPicture(t *testing.T) {
	slide := `<p:sld ` + nsDecls + `><p:cSld><p:spTree>
<p:grpSp><p:grpSpPr><a:xfrm><a:off x="5" y="5"/></a:xfrm></p:grpSpPr>
  <p:sp><p:nvSpPr><p:cNvPr id="4" name="in group"/><p:nvPr/></p:nvSpPr><p:spPr/>
    <p:txBody><a:p><a:r><a:t>grouped text</a:t></a:r></a:p></p:txBody></p:sp>
</p:grpSp>
<p:pic><p:nvPicPr><p:cNvPr id="5" name="Picture 4" descr="a caption"/></p:nvPicPr>
  <p:blipFill><a:blip r:embed="rId2"/></p:blipFill>
  <p:spPr><a:xfrm><a:off x="9" y="9"/></a:xfrm></p:spPr></p:pic>
</p:spTree></p:cSld></p:sld>`
	extra := map[string]string{
		"ppt/slides/_rels/slide1.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/cat.png"/></Relationships>`,
	}
	md := convertDeck(t, minimalDeck(slide, extra))
	if !strings.Contains(md, "grouped text") {
		t.Errorf("group content lost:\n%s", md)
	}
	if !strings.Contains(md, "![a caption](cat.png)") {
		t.Errorf("picture placeholder wrong:\n%s", md)
	}
}

func TestNotes(t *testing.T) {
	slide := `<p:sld ` + nsDecls + `><p:cSld><p:spTree>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="x"/><p:nvPr/></p:nvSpPr><p:spPr/>
  <p:txBody><a:p><a:r><a:t>content</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld></p:sld>`
	notes := `<p:notes ` + nsDecls + `><p:cSld><p:spTree>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Placeholder"/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/>
  <p:txBody><a:p><a:r><a:t>speaker notes here</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld></p:notes>`
	extra := map[string]string{
		"ppt/slides/_rels/slide1.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId7" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide1.xml"/></Relationships>`,
		"ppt/notesSlides/notesSlide1.xml": notes,
	}
	md := convertDeck(t, minimalDeck(slide, extra))
	if !strings.Contains(md, "### Notes:") || !strings.Contains(md, "speaker notes here") {
		t.Errorf("notes missing:\n%s", md)
	}
}

func TestMissingPartsError(t *testing.T) {
	data, size := buildPptx(t, map[string]string{"foo.txt": "not a pptx"})
	if _, _, err := Convert(t.Context(), bytes.NewReader(data), size, 0); err == nil {
		t.Error("expected error for archive without presentation.xml")
	}
}

func FuzzConvert(f *testing.F) {
	slide := `<p:sld ` + nsDecls + `><p:cSld><p:spTree>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="x"/><p:nvPr/></p:nvSpPr><p:spPr/>
  <p:txBody><a:p><a:r><a:t>seed</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld></p:sld>`
	seed, _ := buildPptx(f, minimalDeck(slide, nil))
	f.Add(seed)
	f.Add([]byte("PK\x03\x04 garbage"))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic; errors are fine.
		_, _, _ = Convert(t.Context(), bytes.NewReader(data), int64(len(data)), 0)
	})
}
