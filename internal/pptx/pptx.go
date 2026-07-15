// Package pptx extracts Markdown from PowerPoint (.pptx) files by parsing
// the OOXML parts directly (archive/zip + encoding/xml).
package pptx

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/giraffesyo/downmark/internal/ctxio"
	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/ooxml"
)

var errMissingPart = errors.New("missing archive part")

// Convert reads a .pptx archive and returns its Markdown rendering plus the
// deck title (first title text found, "" if none).
func Convert(ctx context.Context, ra io.ReaderAt, size int64, outputLimit int) (md string, title string, err error) {
	archive, err := ooxml.Open(ctx, ra, size)
	if err != nil {
		return "", "", fmt.Errorf("pptx: not a zip archive: %w", err)
	}
	d := &deck{archive: archive, outputLimit: outputLimit}

	var pres presentation
	if err := d.parseXML(ctx, "ppt/presentation.xml", &pres); err != nil {
		return "", "", err
	}
	presRels, err := d.parseRels(ctx, "ppt/_rels/presentation.xml.rels")
	if err != nil {
		return "", "", err
	}

	b := limitbuf.New(outputLimit)
	for i, sld := range pres.SlideIDs {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		target, ok := presRels[sld.RID]
		if !ok {
			continue
		}
		slidePath := resolveTarget("ppt", target)
		if i > 0 {
			if _, err := b.WriteString("\n"); err != nil {
				return "", "", err
			}
		}
		if _, err := fmt.Fprintf(b, "<!-- Slide number: %d -->\n\n", i+1); err != nil {
			return "", "", err
		}
		slideMD, slideTitle, err := d.convertSlide(ctx, slidePath)
		if err != nil {
			return "", "", fmt.Errorf("pptx: %s: %w", slidePath, err)
		}
		if _, err := b.WriteString(slideMD); err != nil {
			return "", "", err
		}
		if title == "" {
			title = slideTitle
		}
	}
	return b.String(), title, nil
}

type deck struct {
	archive     *ooxml.Archive
	outputLimit int
	err         error
}

func (d *deck) read(ctx context.Context, name string) ([]byte, error) {
	name = strings.TrimPrefix(path.Clean(name), "/")
	data, found, err := d.archive.Read(ctx, name)
	if err != nil {
		d.err = err
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("pptx: %w %q", errMissingPart, name)
	}
	return data, nil
}

func (d *deck) parseXML(ctx context.Context, name string, v any) error {
	data, err := d.read(ctx, name)
	if err != nil {
		return err
	}
	if err := ooxml.ValidateXML(ctx, data); err != nil {
		if ctx.Err() != nil || errors.Is(err, ooxml.ErrXMLComplexity) {
			d.err = err
		}
		return fmt.Errorf("pptx: validate %s: %w", name, err)
	}
	dec := xml.NewDecoder(ctxio.NewReader(ctx, bytes.NewReader(data)))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("pptx: parse %s: %w", name, err)
	}
	return nil
}

// parseRels reads a .rels part into an ID → target map. A missing part
// yields an empty map (slides without relationships are legal).
func (d *deck) parseRels(ctx context.Context, name string) (map[string]string, error) {
	data, err := d.read(ctx, name)
	if err != nil {
		if d.err != nil {
			return nil, d.err
		}
		return map[string]string{}, nil
	}
	var rels relationships
	if err := ooxml.ValidateXML(ctx, data); err != nil {
		return nil, fmt.Errorf("pptx: validate %s: %w", name, err)
	}
	if err := xml.NewDecoder(ctxio.NewReader(ctx, bytes.NewReader(data))).Decode(&rels); err != nil {
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
