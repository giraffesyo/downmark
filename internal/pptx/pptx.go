// Package pptx extracts Markdown from PowerPoint (.pptx) files by parsing
// the OOXML parts directly (archive/zip + encoding/xml).
package pptx

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

// Convert reads a .pptx archive and returns its Markdown rendering plus the
// deck title (first title text found, "" if none).
func Convert(ra io.ReaderAt, size int64) (md string, title string, err error) {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return "", "", fmt.Errorf("pptx: not a zip archive: %w", err)
	}
	d := &deck{zr: zr}

	var pres presentation
	if err := d.parseXML("ppt/presentation.xml", &pres); err != nil {
		return "", "", err
	}
	presRels, err := d.parseRels("ppt/_rels/presentation.xml.rels")
	if err != nil {
		return "", "", err
	}

	var b strings.Builder
	for i, sld := range pres.SlideIDs {
		target, ok := presRels[sld.RID]
		if !ok {
			continue
		}
		slidePath := resolveTarget("ppt", target)
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "<!-- Slide number: %d -->\n\n", i+1)
		slideMD, slideTitle, err := d.convertSlide(slidePath)
		if err != nil {
			return "", "", fmt.Errorf("pptx: %s: %w", slidePath, err)
		}
		b.WriteString(slideMD)
		if title == "" {
			title = slideTitle
		}
	}
	return b.String(), title, nil
}

type deck struct {
	zr *zip.Reader
}

func (d *deck) open(name string) (io.ReadCloser, error) {
	name = strings.TrimPrefix(path.Clean(name), "/")
	for _, f := range d.zr.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("pptx: missing archive part %q", name)
}

func (d *deck) parseXML(name string, v any) error {
	rc, err := d.open(name)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }() // read-only handle
	dec := xml.NewDecoder(rc)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("pptx: parse %s: %w", name, err)
	}
	return nil
}

// parseRels reads a .rels part into an ID → target map. A missing part
// yields an empty map (slides without relationships are legal).
func (d *deck) parseRels(name string) (map[string]string, error) {
	rc, err := d.open(name)
	if err != nil {
		return map[string]string{}, nil
	}
	defer func() { _ = rc.Close() }() // read-only handle
	var rels relationships
	if err := xml.NewDecoder(rc).Decode(&rels); err != nil {
		return nil, fmt.Errorf("pptx: parse %s: %w", name, err)
	}
	m := make(map[string]string, len(rels.Rels))
	for _, r := range rels.Rels {
		m[r.ID] = r.Target
	}
	return m, nil
}

// resolveTarget resolves a relationship target against the directory of the
// part that declared it (base is that directory, e.g. "ppt/slides").
func resolveTarget(base, target string) string {
	if abs, ok := strings.CutPrefix(target, "/"); ok {
		return abs
	}
	return path.Clean(path.Join(base, target))
}

// relsPathFor returns the .rels part path for a given part, e.g.
// ppt/slides/slide1.xml → ppt/slides/_rels/slide1.xml.rels.
func relsPathFor(part string) string {
	return path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
}

type relationships struct {
	Rels []relationship `xml:"Relationship"`
}

type relationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

type presentation struct {
	SlideIDs []slideID `xml:"sldIdLst>sldId"`
}

type slideID struct {
	// r:id must be namespace-qualified: sldId also carries an unqualified
	// numeric id attribute with the same local name.
	RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}
