// Package docx extracts content from Word (.docx) files by parsing the
// OOXML parts directly and emitting intermediate HTML, which the caller
// converts to Markdown via the shared htmlmd backend (mammoth-style
// two-stage conversion).
package docx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"

	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/ooxml"
)

// Options controls conversion behavior.
type Options struct {
	// KeepDataURIs embeds images as full data: URIs instead of filename
	// placeholders.
	KeepDataURIs bool
	// OutputLimit bounds the intermediate HTML rendering in bytes. A
	// non-positive value is unlimited.
	OutputLimit int
}

// Convert parses the archive and returns intermediate HTML plus the
// document title ("" if none).
func Convert(ctx context.Context, ra io.ReaderAt, size int64, opts Options) (html string, title string, err error) {
	archive, err := ooxml.Open(ctx, ra, size)
	if err != nil {
		return "", "", fmt.Errorf("docx: not a zip archive: %w", err)
	}
	d := &doc{archive: archive, opts: opts}

	d.styles, err = parseStyles(ctx, d.readPart(ctx, "word/styles.xml"))
	if err != nil {
		return "", "", err
	}
	d.numbering, err = parseNumbering(ctx, d.readPart(ctx, "word/numbering.xml"))
	if err != nil {
		return "", "", err
	}
	d.rels, err = parseRels(ctx, d.readPart(ctx, "word/_rels/document.xml.rels"))
	if err != nil {
		return "", "", err
	}

	data := d.readPart(ctx, "word/document.xml")
	if d.err != nil {
		return "", "", d.err
	}
	if data == nil {
		return "", "", errors.New("docx: missing word/document.xml")
	}
	root, err := decodeTree(ctx, data)
	if err != nil {
		return "", "", fmt.Errorf("docx: parse word/document.xml: %w", err)
	}
	body := root.descend("document", "body")
	if body == nil {
		return "", "", errors.New("docx: word/document.xml has no body")
	}

	c := &conv{doc: d, b: limitbuf.New(opts.OutputLimit)}
	c.block(ctx, body)
	c.closeLists()
	if d.err != nil {
		return "", "", d.err
	}
	if err := c.b.Err(); err != nil {
		return "", "", err
	}
	if c.err != nil {
		return "", "", c.err
	}
	return c.b.String(), c.title, nil
}

type doc struct {
	archive   *ooxml.Archive
	opts      Options
	err       error
	styles    *styleMap
	numbering *numberingMap
	rels      map[string]rel
}

// readPart returns a zip part's bytes, or nil if absent (most parts are
// optional).
func (d *doc) readPart(ctx context.Context, name string) []byte {
	if d.err != nil {
		return nil
	}
	data, found, err := d.archive.Read(ctx, name)
	if err != nil {
		d.err = fmt.Errorf("docx: read %s: %w", name, err)
		return nil
	}
	if !found {
		return nil
	}
	return data
}

// imageSrc resolves a relationship ID to an <img> src: the media file's
// base name, or a full data URI when KeepDataURIs is set.
func (d *doc) imageSrc(ctx context.Context, relID string) string {
	r, ok := d.rels[relID]
	if !ok {
		return ""
	}
	target := strings.TrimPrefix(r.target, "/")
	if !strings.HasPrefix(target, "word/") && !strings.HasPrefix(r.target, "/") {
		target = path.Clean(path.Join("word", r.target))
	}
	if !d.opts.KeepDataURIs {
		return path.Base(target)
	}
	data := d.readPart(ctx, target)
	if data == nil {
		return path.Base(target)
	}
	mtype := mime.TypeByExtension(path.Ext(target))
	if mtype == "" {
		mtype = "application/octet-stream"
	}
	return "data:" + mtype + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// node is a generic XML tree node matched by local element name; OOXML
// producers vary namespace prefixes, so prefixes are never consulted.
type node struct {
	name  string
	attrs []xml.Attr
	kids  []*node
	text  strings.Builder
}

func decodeTree(ctx context.Context, data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	root := &node{name: "#root"}
	stack := []*node{root}
	elements, attributes := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			elements++
			attributes += len(t.Attr)
			if elements > ooxml.MaxXMLElements || attributes > ooxml.MaxXMLAttributes || len(stack) > ooxml.MaxXMLDepth {
				return nil, fmt.Errorf("docx: %w", ooxml.ErrXMLComplexity)
			}
			n := &node{name: t.Name.Local, attrs: t.Attr}
			parent := stack[len(stack)-1]
			parent.kids = append(parent.kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			stack[len(stack)-1].text.Write(t)
		}
	}
	return root, nil
}

func (n *node) child(name string) *node {
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

// attr returns the attribute with the given local name, regardless of
// namespace.
func (n *node) attr(name string) string {
	for _, a := range n.attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// descend returns the first descendant chain matching the given names.
func (n *node) descend(names ...string) *node {
	cur := n
	for _, name := range names {
		if cur = cur.child(name); cur == nil {
			return nil
		}
	}
	return cur
}

// eachDescendant walks the subtree in document order.
func (n *node) eachDescendant(fn func(*node)) {
	for _, k := range n.kids {
		fn(k)
		k.eachDescendant(fn)
	}
}
