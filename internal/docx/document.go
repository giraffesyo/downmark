package docx

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/giraffesyo/downmark/internal/limitbuf"
)

// conv walks the document body and emits intermediate HTML.
type conv struct {
	doc   *doc
	b     *limitbuf.Buffer
	title string
	err   error

	// lists tracks currently open <ul>/<ol> levels; each level may have an
	// open <li> that wraps any nested sublist.
	lists []openList
}

type openList struct {
	ordered bool
	liOpen  bool
}

// block processes body-level children: paragraphs, tables, and containers
// that wrap them.
func (c *conv) block(ctx context.Context, n *node) {
	for _, kid := range n.kids {
		if err := ctx.Err(); err != nil {
			c.err = err
			return
		}
		switch kid.name {
		case "p":
			c.paragraph(ctx, kid)
		case "tbl":
			c.closeLists()
			c.table(ctx, kid)
		case "sdt":
			if content := kid.child("sdtContent"); content != nil {
				c.block(ctx, content)
			}
		case "sectPr":
			// Section properties carry no content.
		}
	}
}

func (c *conv) paragraph(ctx context.Context, p *node) {
	pPr := p.child("pPr")
	styleID := ""
	if pPr != nil {
		if s := pPr.child("pStyle"); s != nil {
			styleID = s.attr("val")
		}
	}

	inline := c.inlineHTML(ctx, p)

	// Heading?
	level := c.doc.styles.headingLevel(styleID)
	if level == 0 && pPr != nil {
		if o := pPr.child("outlineLvl"); o != nil {
			if v, err := strconv.Atoi(o.attr("val")); err == nil && v >= 0 && v < 9 {
				level = v + 1
			}
		}
	}
	if level > 0 {
		if strings.TrimSpace(inline) == "" {
			return
		}
		c.closeLists()
		if c.title == "" && (c.doc.styles.isTitle(styleID) || level == 1) {
			c.title = strings.TrimSpace(textOnly(p))
		}
		c.writef("<h%d>%s</h%d>\n", level, inline, level)
		return
	}

	// List item?
	if pPr != nil {
		if numPr := pPr.child("numPr"); numPr != nil {
			numID, ilvlStr := "", "0"
			if n := numPr.child("numId"); n != nil {
				numID = n.attr("val")
			}
			if n := numPr.child("ilvl"); n != nil {
				ilvlStr = n.attr("val")
			}
			if ordered, ok := c.doc.numbering.listKind(numID, ilvlStr); ok {
				ilvl, _ := strconv.Atoi(ilvlStr)
				c.listItem(ilvl, ordered, inline)
				return
			}
		}
	}

	// Plain paragraph.
	c.closeLists()
	if strings.TrimSpace(inline) == "" {
		return
	}
	c.writef("<p>%s</p>\n", inline)
}

// listItem adjusts the open-list stack to the item's level and emits it.
func (c *conv) listItem(ilvl int, ordered bool, inner string) {
	if ilvl < 0 {
		ilvl = 0
	}
	// Close deeper levels.
	for len(c.lists) > ilvl+1 {
		c.popList()
	}
	// Same level but different type: restart the list.
	if len(c.lists) == ilvl+1 && c.lists[len(c.lists)-1].ordered != ordered {
		c.popList()
	}
	// Open levels up to the target.
	for len(c.lists) < ilvl+1 {
		tag := "ul"
		if ordered {
			tag = "ol"
		}
		c.writeString("<" + tag + ">\n")
		c.lists = append(c.lists, openList{ordered: ordered})
	}
	top := &c.lists[len(c.lists)-1]
	if top.liOpen {
		c.writeString("</li>\n")
	}
	c.writeString("<li>" + inner)
	top.liOpen = true
}

func (c *conv) popList() {
	top := c.lists[len(c.lists)-1]
	if top.liOpen {
		c.writeString("</li>\n")
	}
	if top.ordered {
		c.writeString("</ol>\n")
	} else {
		c.writeString("</ul>\n")
	}
	c.lists = c.lists[:len(c.lists)-1]
	// The enclosing <li> (if any) wrapped this sublist; it stays open until
	// its own level advances.
}

func (c *conv) closeLists() {
	for len(c.lists) > 0 {
		c.popList()
	}
}

// inlineHTML renders a paragraph's inline content (runs, links, images).
func (c *conv) inlineHTML(ctx context.Context, p *node) string {
	var b strings.Builder
	c.inlineChildren(ctx, &b, p)
	return b.String()
}

func (c *conv) inlineChildren(ctx context.Context, b *strings.Builder, n *node) {
	for _, kid := range n.kids {
		if err := ctx.Err(); err != nil {
			c.err = err
			return
		}
		switch kid.name {
		case "r":
			c.run(ctx, b, kid)
		case "hyperlink":
			c.hyperlink(ctx, b, kid)
		case "ins", "smartTag", "fldSimple":
			c.inlineChildren(ctx, b, kid)
		case "sdt":
			if content := kid.child("sdtContent"); content != nil {
				c.inlineChildren(ctx, b, content)
			}
		case "oMath", "oMathPara":
			// Math degrades to its plain text (OMML → LaTeX is a later
			// enhancement).
			var text strings.Builder
			kid.eachDescendant(func(d *node) {
				if d.name == "t" {
					text.WriteString(d.text.String())
				}
			})
			b.WriteString(html.EscapeString(text.String()))
		case "del", "pPr", "bookmarkStart", "bookmarkEnd", "proofErr",
			"commentRangeStart", "commentRangeEnd", "commentReference":
			// Deleted content and non-content markers.
		}
	}
}

func (c *conv) hyperlink(ctx context.Context, b *strings.Builder, link *node) {
	href := ""
	if id := link.attr("id"); id != "" {
		if r, ok := c.doc.rels[id]; ok && r.external {
			href = r.target
		}
	}
	if href == "" {
		// Internal anchors degrade to plain content.
		c.inlineChildren(ctx, b, link)
		return
	}
	b.WriteString(`<a href="` + html.EscapeString(href) + `">`)
	c.inlineChildren(ctx, b, link)
	b.WriteString("</a>")
}

func (c *conv) run(ctx context.Context, b *strings.Builder, r *node) {
	var opening, closing string
	if rPr := r.child("rPr"); rPr != nil {
		if flagOn(rPr, "b") {
			opening, closing = opening+"<strong>", "</strong>"+closing
		}
		if flagOn(rPr, "i") {
			opening, closing = opening+"<em>", "</em>"+closing
		}
		if flagOn(rPr, "strike") {
			opening, closing = opening+"<del>", "</del>"+closing
		}
		if v := rPr.child("vertAlign"); v != nil {
			switch v.attr("val") {
			case "superscript":
				opening, closing = opening+"<sup>", "</sup>"+closing
			case "subscript":
				opening, closing = opening+"<sub>", "</sub>"+closing
			}
		}
	}

	var content strings.Builder
	for _, kid := range r.kids {
		switch kid.name {
		case "t":
			content.WriteString(html.EscapeString(kid.text.String()))
		case "tab":
			content.WriteString(" ")
		case "br", "cr":
			content.WriteString("<br/>")
		case "noBreakHyphen":
			content.WriteString("-")
		case "drawing", "pict", "object":
			c.image(ctx, &content, kid)
		}
	}
	if content.Len() == 0 {
		return
	}
	b.WriteString(opening)
	b.WriteString(content.String())
	b.WriteString(closing)
}

// flagOn reports whether a run-property toggle like w:b is present and not
// explicitly disabled (w:val="0"/"false").
func flagOn(rPr *node, name string) bool {
	f := rPr.child(name)
	if f == nil {
		return false
	}
	switch strings.ToLower(f.attr("val")) {
	case "0", "false", "none", "off":
		return false
	}
	return true
}

// image renders the first embedded picture found under a drawing node.
func (c *conv) image(ctx context.Context, b *strings.Builder, drawing *node) {
	var embed, alt string
	drawing.eachDescendant(func(d *node) {
		switch d.name {
		case "blip":
			if embed == "" {
				embed = d.attr("embed")
			}
		case "docPr":
			if alt == "" {
				alt = d.attr("descr")
				if alt == "" {
					alt = d.attr("name")
				}
			}
		}
	})
	if embed == "" {
		return
	}
	src := c.doc.imageSrc(ctx, embed)
	if src == "" {
		return
	}
	fmt.Fprintf(b, `<img src="%s" alt="%s"/>`, html.EscapeString(src), html.EscapeString(alt))
}

func (c *conv) table(ctx context.Context, t *node) {
	c.writeString("<table>\n")
	firstRow := true
	for _, tr := range t.kids {
		if err := ctx.Err(); err != nil {
			c.err = err
			return
		}
		if tr.name != "tr" {
			continue
		}
		c.writeString("<tr>")
		tag := "td"
		if firstRow {
			tag = "th"
		}
		for _, tc := range tr.kids {
			if tc.name != "tc" {
				continue
			}
			span := 1
			merged := false
			if tcPr := tc.child("tcPr"); tcPr != nil {
				if g := tcPr.child("gridSpan"); g != nil {
					if v, err := strconv.Atoi(g.attr("val")); err == nil && v > 1 {
						span = v
					}
				}
				if vm := tcPr.child("vMerge"); vm != nil && vm.attr("val") != "restart" {
					merged = true
				}
			}
			content := ""
			if !merged {
				content = c.cellHTML(ctx, tc)
			}
			c.writeString("<" + tag + ">" + content + "</" + tag + ">")
			for range span - 1 {
				c.writeString("<" + tag + "></" + tag + ">")
			}
		}
		c.writeString("</tr>\n")
		firstRow = false
	}
	c.writeString("</table>\n")
}

func (c *conv) writeString(s string) {
	_, _ = c.b.WriteString(s) // Buffer retains the size error for Convert.
}

func (c *conv) writef(format string, args ...any) {
	_, _ = fmt.Fprintf(c.b, format, args...) // Buffer retains the size error for Convert.
}

// cellHTML renders a table cell: its paragraphs joined by <br/>; nested
// tables are flattened to their text.
func (c *conv) cellHTML(ctx context.Context, tc *node) string {
	var parts []string
	var walk func(n *node)
	walk = func(n *node) {
		if err := ctx.Err(); err != nil {
			c.err = err
			return
		}
		for _, kid := range n.kids {
			switch kid.name {
			case "p":
				if inner := c.inlineHTML(ctx, kid); strings.TrimSpace(inner) != "" {
					parts = append(parts, inner)
				}
			case "tbl":
				if text := strings.TrimSpace(textOnly(kid)); text != "" {
					parts = append(parts, html.EscapeString(text))
				}
			case "sdt":
				if content := kid.child("sdtContent"); content != nil {
					walk(content)
				}
			}
		}
	}
	walk(tc)
	return strings.Join(parts, "<br/>")
}

// textOnly returns the concatenated w:t text of a subtree.
func textOnly(n *node) string {
	var b strings.Builder
	n.eachDescendant(func(d *node) {
		if d.name == "t" {
			b.WriteString(d.text.String())
		}
	})
	return b.String()
}
