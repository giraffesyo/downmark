package docx

import (
	"context"
	"errors"

	"github.com/giraffesyo/downmark/internal/ooxml"
)

type numberingMap struct {
	numToAbstract map[string]string
	abstractFmt   map[string]map[string]string // abstractNumId → ilvl → numFmt
}

func parseNumbering(ctx context.Context, data []byte) (*numberingMap, error) {
	nm := &numberingMap{
		numToAbstract: map[string]string{},
		abstractFmt:   map[string]map[string]string{},
	}
	if data == nil {
		return nm, nil
	}
	root, err := decodeTree(ctx, data)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, ooxml.ErrXMLComplexity) {
			return nil, err
		}
		return nm, nil
	}
	numbering := root.child("numbering")
	if numbering == nil {
		return nm, nil
	}
	for _, n := range numbering.kids {
		switch n.name {
		case "abstractNum":
			id := n.attr("abstractNumId")
			lvls := map[string]string{}
			for _, lvl := range n.kids {
				if lvl.name != "lvl" {
					continue
				}
				if f := lvl.child("numFmt"); f != nil {
					lvls[lvl.attr("ilvl")] = f.attr("val")
				}
			}
			nm.abstractFmt[id] = lvls
		case "num":
			if a := n.child("abstractNumId"); a != nil {
				nm.numToAbstract[n.attr("numId")] = a.attr("val")
			}
		}
	}
	return nm, nil
}

// listKind resolves a numbering reference. ok is false when the numId does
// not resolve to a real list (notably numId 0, "no numbering").
func (nm *numberingMap) listKind(numID, ilvl string) (ordered bool, ok bool) {
	if numID == "" || numID == "0" {
		return false, false
	}
	abstract, found := nm.numToAbstract[numID]
	if !found {
		return false, false
	}
	format := nm.abstractFmt[abstract][ilvl]
	if format == "" {
		// A resolvable num with no level format still marks a list item.
		return false, true
	}
	return format != "bullet", true
}
