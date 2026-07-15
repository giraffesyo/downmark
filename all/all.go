// Package all wires every downmark converter into one engine, for
// consumers who want the full format matrix and don't mind linking every
// converter's dependencies. Import individual convert/... packages instead
// to keep binaries small.
package all

import (
	"context"
	"io"
	"sync"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/convert/csv"
	"github.com/giraffesyo/downmark/convert/docx"
	"github.com/giraffesyo/downmark/convert/html"
	"github.com/giraffesyo/downmark/convert/pdf"
	"github.com/giraffesyo/downmark/convert/pptx"
	"github.com/giraffesyo/downmark/convert/xlsx"
	"github.com/giraffesyo/downmark/convert/zipfile"
)

// Options configures cross-format conversion behavior.
type Options struct {
	// KeepDataURIs preserves full data: URIs in output (HTML) and embeds
	// images as data URIs (DOCX) instead of short placeholders.
	KeepDataURIs bool
}

// New returns an engine with every converter registered.
func New(opts Options) *downmark.Engine {
	e := downmark.New()
	html.Register(e, html.Options{KeepDataURIs: opts.KeepDataURIs})
	csv.Register(e)
	xlsx.Register(e)
	pdf.Register(e)
	pptx.Register(e)
	docx.Register(e, docx.Options{KeepDataURIs: opts.KeepDataURIs})
	zipfile.Register(e)
	return e
}

var defaultEngine = sync.OnceValue(func() *downmark.Engine { return New(Options{}) })

// Convert converts r using a shared default engine with every converter.
func Convert(ctx context.Context, r io.Reader, hints downmark.StreamInfo) (*downmark.Result, error) {
	return defaultEngine().Convert(ctx, r, hints)
}

// ConvertFile converts the file at path using a shared default engine with
// every converter.
func ConvertFile(ctx context.Context, path string) (*downmark.Result, error) {
	return defaultEngine().ConvertFile(ctx, path)
}
