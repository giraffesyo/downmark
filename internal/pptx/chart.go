package pptx

import (
	"context"
	"encoding/xml"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/giraffesyo/downmark/internal/limitbuf"
	"github.com/giraffesyo/downmark/internal/mdutil"
)

type chartSpace struct {
	Chart struct {
		Title *struct {
			Tx *struct {
				Rich *txBody `xml:"rich"`
			} `xml:"tx"`
		} `xml:"title"`
		PlotArea plotArea `xml:"plotArea"`
	} `xml:"chart"`
}

// plotArea collects c:ser elements wherever they appear: the parent element
// name varies by chart kind (c:barChart, c:lineChart, c:pieChart, ...).
type plotArea struct {
	Series []series
}

func (pa *plotArea) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "ser" {
				var s series
				if err := d.DecodeElement(&s, &t); err != nil {
					return err
				}
				pa.Series = append(pa.Series, s)
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return nil
			}
		}
	}
}

type series struct {
	Tx struct {
		StrRef *cacheRef `xml:"strRef"`
	} `xml:"tx"`
	Cat *dataRef `xml:"cat"`
	Val *dataRef `xml:"val"`
}

type dataRef struct {
	NumRef *cacheRef `xml:"numRef"`
	StrRef *cacheRef `xml:"strRef"`
}

func (dr *dataRef) points() map[int]string {
	if dr == nil {
		return nil
	}
	for _, ref := range []*cacheRef{dr.StrRef, dr.NumRef} {
		if ref == nil {
			continue
		}
		pts := ref.pointMap()
		if len(pts) > 0 {
			return pts
		}
	}
	return nil
}

type cacheRef struct {
	NumPts []cachePt `xml:"numCache>pt"`
	StrPts []cachePt `xml:"strCache>pt"`
}

func (cr *cacheRef) pointMap() map[int]string {
	m := map[int]string{}
	for _, pts := range [][]cachePt{cr.StrPts, cr.NumPts} {
		for _, p := range pts {
			m[p.Idx] = p.V
		}
	}
	return m
}

type cachePt struct {
	Idx int    `xml:"idx,attr"`
	V   string `xml:"v"`
}

// renderChart parses the chart part and renders its title plus a data
// table. A chart that cannot be parsed degrades to a placeholder.
func (d *deck) renderChart(ctx context.Context, slidePath, target string) string {
	const fallback = "[unsupported chart]"
	if target == "" {
		return fallback
	}
	var cs chartSpace
	if err := d.parseXML(ctx, resolveTarget(path.Dir(slidePath), target), &cs); err != nil {
		return fallback
	}

	heading := "### Chart"
	if t := cs.Chart.Title; t != nil && t.Tx != nil {
		if name := t.Tx.Rich.text(); name != "" {
			heading = "### Chart: " + strings.ReplaceAll(name, "\n", " ")
		}
	}

	sers := cs.Chart.PlotArea.Series
	if len(sers) == 0 {
		return heading
	}

	header := []string{"Category"}
	valuesBySer := make([]map[int]string, len(sers))
	var cats map[int]string
	idxSet := map[int]bool{}
	for i, s := range sers {
		name := ""
		if s.Tx.StrRef != nil {
			for _, v := range s.Tx.StrRef.pointMap() {
				name = v
				break
			}
		}
		if name == "" {
			name = "Series " + strconv.Itoa(i+1)
		}
		header = append(header, name)
		if cats == nil {
			cats = s.Cat.points()
		}
		valuesBySer[i] = s.Val.points()
		for idx := range valuesBySer[i] {
			idxSet[idx] = true
		}
	}
	for idx := range cats {
		idxSet[idx] = true
	}
	idxs := make([]int, 0, len(idxSet))
	for idx := range idxSet {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)

	rows := [][]string{header}
	for _, idx := range idxs {
		row := []string{cats[idx]}
		for i := range sers {
			row = append(row, valuesBySer[i][idx])
		}
		rows = append(rows, row)
	}
	b := limitbuf.New(d.outputLimit)
	if _, err := b.WriteString(heading + "\n\n"); err != nil {
		d.err = err
		return fallback
	}
	if err := mdutil.WriteTable(b, rows); err != nil {
		d.err = err
		return fallback
	}
	return b.String()
}
