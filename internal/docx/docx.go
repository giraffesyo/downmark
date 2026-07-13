// Package docx extracts content from Word (.docx) files by parsing the
// OOXML parts directly and emitting intermediate HTML, which the caller
// converts to Markdown via the shared htmlmd backend (mammoth-style
// two-stage conversion).
package docx

import (
	"archive/zip"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"
)

// Options controls conversion behavior.
type Options struct {
	// KeepDataURIs embeds images as full data: URIs instead of filename
	// placeholders.
	KeepDataURIs bool
}

// Convert parses the archive and returns intermediate HTML plus the
// document title ("" if none).
func Convert(ra io.ReaderAt, size int64, opts Options) (html string, title string, err error) {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return "", "", fmt.Errorf("docx: not a zip archive: %w", err)
	}
	d := &doc{zr: zr, opts: opts}

	d.styles = parseStyles(d.readPart("word/styles.xml"))
	d.numbering = parseNumbering(d.readPart("word/numbering.xml"))
	d.rels = parseRels(d.readPart("word/_rels/document.xml.rels"))

	data := d.readPart("word/document.xml")
	if data == nil {
		return "", "", errors.New("docx: missing word/document.xml")
	}
	root, err := decodeTree(data)
	if err != nil {
		return "", "", fmt.Errorf("docx: parse word/document.xml: %w", err)
	}
	body := root.descend("document", "body")
	if body == nil {
		return "", "", errors.New("docx: word/document.xml has no body")
	}

	c := &conv{doc: d}
	c.block(body)
	c.closeLists()
	return c.b.String(), c.title, nil
}

type doc struct {
	zr        *zip.Reader
	opts      Options
	styles    *styleMap
	numbering *numberingMap
	rels      map[string]rel
}

// readPart returns a zip part's bytes, or nil if absent (most parts are
// optional).
func (d *doc) readPart(name string) []byte {
	for _, f := range d.zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer func() { _ = rc.Close() }() // read-only handle
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil
			}
			return data
		}
	}
	return nil
}

// imageSrc resolves a relationship ID to an <img> src: the media file's
// base name, or a full data URI when KeepDataURIs is set.
func (d *doc) imageSrc(relID string) string {
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
	data := d.readPart(target)
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

func decodeTree(data []byte) (*node, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	root := &node{name: "#root"}
	stack := []*node{root}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
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
