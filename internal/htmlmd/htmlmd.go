// Package htmlmd is the shared HTML-to-Markdown backend used by the HTML
// and DOCX converters.
package htmlmd

import (
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/giraffesyo/downmark/internal/ctxio"
)

// Options controls conversion behavior.
type Options struct {
	// KeepDataURIs preserves full data: URIs instead of truncating them.
	KeepDataURIs bool
}

// Convert parses HTML from r (which must already yield UTF-8) and returns
// Markdown plus the <title> text ("" if absent).
func Convert(ctx context.Context, r io.Reader, opts Options) (md string, title string, err error) {
	doc, err := html.Parse(ctxio.NewReader(ctx, r))
	if err != nil {
		return "", "", err
	}
	return convertDoc(ctx, doc, opts)
}

// ConvertString converts an HTML string (UTF-8).
func ConvertString(ctx context.Context, s string, opts Options) (md string, title string, err error) {
	return Convert(ctx, strings.NewReader(s), opts)
}

func convertDoc(ctx context.Context, doc *html.Node, opts Options) (md string, title string, err error) {
	title, err = applyPolicy(ctx, doc, opts)
	if err != nil {
		return "", "", err
	}

	root := doc
	if body := findElement(doc, atom.Body); body != nil {
		root = body
	}

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
			strikethrough.NewStrikethroughPlugin(),
		),
	)
	out, err := conv.ConvertNode(root, converter.WithContext(ctx))
	if err != nil {
		return "", "", err
	}
	return string(out), title, nil
}

// applyPolicy mutates the DOM in place — dropping script/style/noscript and
// comments, unwrapping unsafe links, truncating data: image URIs — and
// returns the document title.
func applyPolicy(ctx context.Context, doc *html.Node, opts Options) (title string, err error) {
	var walk func(n *html.Node) error
	walk = func(n *html.Node) error {
		for c := n.FirstChild; c != nil; {
			if err := ctx.Err(); err != nil {
				return err
			}
			next := c.NextSibling
			switch c.Type {
			case html.CommentNode:
				n.RemoveChild(c)
			case html.ElementNode:
				switch c.DataAtom {
				case atom.Script, atom.Style, atom.Noscript:
					n.RemoveChild(c)
				case atom.Title:
					if title == "" {
						title = collapseSpace(textContent(c))
					}
					if err := walk(c); err != nil {
						return err
					}
				case atom.A:
					if !safeLink(attr(c, "href")) {
						unwrap(n, c)
					} else {
						if err := walk(c); err != nil {
							return err
						}
					}
				case atom.Img:
					if !opts.KeepDataURIs {
						truncateDataURI(c)
					}
					if err := walk(c); err != nil {
						return err
					}
				default:
					if err := walk(c); err != nil {
						return err
					}
				}
			default:
				if err := walk(c); err != nil {
					return err
				}
			}
			c = next
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return "", err
	}
	return title, nil
}

// safeLink allows relative URLs and the http, https, file, and mailto
// schemes; everything else (javascript:, vbscript:, ...) is rejected.
func safeLink(href string) bool {
	if href == "" {
		return true
	}
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "", "http", "https", "file", "mailto":
		return true
	}
	return false
}

// unwrap replaces child with its own children, preserving link text.
func unwrap(parent, child *html.Node) {
	for c := child.FirstChild; c != nil; {
		next := c.NextSibling
		child.RemoveChild(c)
		parent.InsertBefore(c, child)
		c = next
	}
	parent.RemoveChild(child)
}

func truncateDataURI(img *html.Node) {
	for i, a := range img.Attr {
		if a.Key != "src" {
			continue
		}
		if strings.HasPrefix(a.Val, "data:") {
			if head, _, found := strings.Cut(a.Val, ","); found {
				img.Attr[i].Val = head + ",..."
			}
		}
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func findElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, a); found != nil {
			return found
		}
	}
	return nil
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
