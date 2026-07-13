package pptx

import (
	"encoding/xml"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/giraffesyo/downmark/internal/mdutil"
)

type slideXML struct {
	CSld struct {
		SpTree shapeTree `xml:"spTree"`
	} `xml:"cSld"`
}

// shapeTree models p:spTree and p:grpSp: the container of a slide's shapes.
// Relative order across different element types is irrelevant because
// shapes are re-sorted by position, like upstream markitdown.
type shapeTree struct {
	GrpSpPr struct {
		Xfrm *xfrm `xml:"xfrm"`
	} `xml:"grpSpPr"`
	Shapes []shape        `xml:"sp"`
	Pics   []pic          `xml:"pic"`
	Frames []graphicFrame `xml:"graphicFrame"`
	Groups []shapeTree    `xml:"grpSp"`
}

type shape struct {
	NvSpPr struct {
		CNvPr cNvPr `xml:"cNvPr"`
		NvPr  struct {
			Ph *placeholder `xml:"ph"`
		} `xml:"nvPr"`
	} `xml:"nvSpPr"`
	SpPr struct {
		Xfrm *xfrm `xml:"xfrm"`
	} `xml:"spPr"`
	TxBody *txBody `xml:"txBody"`
}

type placeholder struct {
	Type string `xml:"type,attr"`
}

type cNvPr struct {
	Name  string `xml:"name,attr"`
	Descr string `xml:"descr,attr"`
}

type pic struct {
	NvPicPr struct {
		CNvPr cNvPr `xml:"cNvPr"`
	} `xml:"nvPicPr"`
	BlipFill struct {
		Blip struct {
			Embed string `xml:"embed,attr"`
		} `xml:"blip"`
	} `xml:"blipFill"`
	SpPr struct {
		Xfrm *xfrm `xml:"xfrm"`
	} `xml:"spPr"`
}

type graphicFrame struct {
	Xfrm    *xfrm `xml:"xfrm"`
	Graphic struct {
		Data struct {
			Tbl   *tbl `xml:"tbl"`
			Chart *struct {
				RID string `xml:"id,attr"`
			} `xml:"chart"`
		} `xml:"graphicData"`
	} `xml:"graphic"`
}

type tbl struct {
	Rows []struct {
		Cells []struct {
			TxBody *txBody `xml:"txBody"`
		} `xml:"tc"`
	} `xml:"tr"`
}

type xfrm struct {
	Off *struct {
		X int64 `xml:"x,attr"`
		Y int64 `xml:"y,attr"`
	} `xml:"off"`
}

func (x *xfrm) pos() (int64, int64) {
	if x == nil || x.Off == nil {
		// Shapes without an explicit position sort first (upstream's -inf).
		return math.MinInt64, math.MinInt64
	}
	return x.Off.Y, x.Off.X
}

// txBody collects the text of a p:txBody / c:rich element: one entry per
// a:p paragraph, with a:br rendered as embedded newlines. A custom
// unmarshaler is needed because runs and breaks interleave.
type txBody struct {
	Paragraphs []string
}

func (tb *txBody) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var cur strings.Builder
	inPara, inT := false, 0
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inPara = true
				cur.Reset()
			case "br":
				if inPara {
					cur.WriteByte('\n')
				}
			case "t":
				inT++
			case "bodyPr", "lstStyle", "pPr", "rPr", "defRPr":
				if err := d.Skip(); err != nil {
					return err
				}
			}
		case xml.CharData:
			if inT > 0 {
				cur.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inT--
			case "p":
				if inPara {
					tb.Paragraphs = append(tb.Paragraphs, cur.String())
					inPara = false
				}
			}
			if t.Name == start.Name {
				return nil
			}
		}
	}
}

func (tb *txBody) text() string {
	if tb == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(tb.Paragraphs, "\n"))
}

// item is one rendered shape with its sort position.
type item struct {
	y, x int64
	md   string
}

func (d *deck) convertSlide(slidePath string) (md string, title string, err error) {
	var sld slideXML
	if err := d.parseXML(slidePath, &sld); err != nil {
		return "", "", err
	}
	rels, err := d.parseRels(relsPathFor(slidePath))
	if err != nil {
		return "", "", err
	}

	items, title := d.renderTree(sld.CSld.SpTree, slidePath, rels)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].y != items[j].y {
			return items[i].y < items[j].y
		}
		return items[i].x < items[j].x
	})

	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.md)
		b.WriteString("\n\n")
	}

	if notes := d.slideNotes(slidePath, rels); notes != "" {
		b.WriteString("### Notes:\n\n")
		b.WriteString(notes)
		b.WriteString("\n\n")
	}
	return b.String(), title, nil
}

// renderTree renders every shape in a tree into positioned items. Groups
// become a single item positioned at the group's offset, with their
// children sorted internally.
func (d *deck) renderTree(tree shapeTree, slidePath string, rels map[string]string) (items []item, title string) {
	for _, sp := range tree.Shapes {
		text := sp.TxBody.text()
		if text == "" {
			continue
		}
		y, x := sp.SpPr.Xfrm.pos()
		md := text
		if ph := sp.NvSpPr.NvPr.Ph; ph != nil && (ph.Type == "title" || ph.Type == "ctrTitle") {
			md = "# " + strings.ReplaceAll(text, "\n", " ")
			if title == "" {
				title = strings.ReplaceAll(text, "\n", " ")
			}
		}
		items = append(items, item{y: y, x: x, md: md})
	}

	for _, p := range tree.Pics {
		y, x := p.SpPr.Xfrm.pos()
		alt := p.NvPicPr.CNvPr.Descr
		if alt == "" {
			alt = p.NvPicPr.CNvPr.Name
		}
		name := "image"
		if target, ok := rels[p.BlipFill.Blip.Embed]; ok {
			name = path.Base(target)
		}
		items = append(items, item{y: y, x: x, md: fmt.Sprintf("![%s](%s)", mdutil.EscapeCell(alt), name)})
	}

	for _, f := range tree.Frames {
		y, x := f.Xfrm.pos()
		switch {
		case f.Graphic.Data.Tbl != nil:
			if md := renderTable(f.Graphic.Data.Tbl); md != "" {
				items = append(items, item{y: y, x: x, md: md})
			}
		case f.Graphic.Data.Chart != nil:
			md := d.renderChart(slidePath, rels[f.Graphic.Data.Chart.RID])
			items = append(items, item{y: y, x: x, md: md})
		}
	}

	for _, g := range tree.Groups {
		sub, subTitle := d.renderTree(g, slidePath, rels)
		if title == "" {
			title = subTitle
		}
		if len(sub) == 0 {
			continue
		}
		sort.SliceStable(sub, func(i, j int) bool {
			if sub[i].y != sub[j].y {
				return sub[i].y < sub[j].y
			}
			return sub[i].x < sub[j].x
		})
		parts := make([]string, len(sub))
		for i, it := range sub {
			parts[i] = it.md
		}
		y, x := g.GrpSpPr.Xfrm.pos()
		items = append(items, item{y: y, x: x, md: strings.Join(parts, "\n\n")})
	}
	return items, title
}

func renderTable(t *tbl) string {
	var rows [][]string
	for _, tr := range t.Rows {
		var row []string
		for _, tc := range tr.Cells {
			row = append(row, strings.Join(tc.TxBody.paragraphsOrEmpty(), " "))
		}
		rows = append(rows, row)
	}
	return mdutil.Table(rows)
}

func (tb *txBody) paragraphsOrEmpty() []string {
	if tb == nil {
		return nil
	}
	return tb.Paragraphs
}

// slideNotes returns the text of the slide's notes page, if any.
func (d *deck) slideNotes(slidePath string, rels map[string]string) string {
	for _, target := range rels {
		if !strings.Contains(target, "notesSlide") {
			continue
		}
		notesPath := resolveTarget(path.Dir(slidePath), target)
		var sld slideXML
		if err := d.parseXML(notesPath, &sld); err != nil {
			return ""
		}
		var bodyTexts, allTexts []string
		var collect func(tree shapeTree)
		collect = func(tree shapeTree) {
			for _, sp := range tree.Shapes {
				text := sp.TxBody.text()
				if text == "" {
					continue
				}
				allTexts = append(allTexts, text)
				if ph := sp.NvSpPr.NvPr.Ph; ph != nil && ph.Type == "body" {
					bodyTexts = append(bodyTexts, text)
				}
			}
			for _, g := range tree.Groups {
				collect(g)
			}
		}
		collect(sld.CSld.SpTree)
		if len(bodyTexts) > 0 {
			return strings.Join(bodyTexts, "\n\n")
		}
		return strings.Join(allTexts, "\n\n")
	}
	return ""
}
