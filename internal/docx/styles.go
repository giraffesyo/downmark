package docx

import (
	"regexp"
	"strconv"
	"strings"
)

type styleInfo struct {
	name       string
	basedOn    string
	outlineLvl int // -1 if unset
}

type styleMap struct {
	byID map[string]*styleInfo
}

func parseStyles(data []byte) *styleMap {
	sm := &styleMap{byID: map[string]*styleInfo{}}
	if data == nil {
		return sm
	}
	root, err := decodeTree(data)
	if err != nil {
		return sm
	}
	styles := root.child("styles")
	if styles == nil {
		return sm
	}
	for _, st := range styles.kids {
		if st.name != "style" || st.attr("type") != "paragraph" {
			continue
		}
		id := st.attr("styleId")
		if id == "" {
			continue
		}
		info := &styleInfo{outlineLvl: -1}
		if n := st.child("name"); n != nil {
			info.name = n.attr("val")
		}
		if n := st.child("basedOn"); n != nil {
			info.basedOn = n.attr("val")
		}
		if n := st.descend("pPr", "outlineLvl"); n != nil {
			if v, err := strconv.Atoi(n.attr("val")); err == nil {
				info.outlineLvl = v
			}
		}
		sm.byID[id] = info
	}
	return sm
}

var headingName = regexp.MustCompile(`(?i)^heading\s*([1-9])$`)

// headingLevel resolves a paragraph style to a heading level 1-9, or 0 if
// the style is not a heading. basedOn chains are followed (bounded).
func (sm *styleMap) headingLevel(styleID string) int {
	id := styleID
	for range 10 {
		if lvl := headingLevelOf(id, sm.byID[id]); lvl > 0 {
			return lvl
		}
		info := sm.byID[id]
		if info == nil || info.basedOn == "" {
			return 0
		}
		id = info.basedOn
	}
	return 0
}

func headingLevelOf(id string, info *styleInfo) int {
	names := []string{id}
	if info != nil {
		names = append(names, info.name)
	}
	for _, n := range names {
		if m := headingName.FindStringSubmatch(strings.TrimSpace(n)); m != nil {
			lvl, _ := strconv.Atoi(m[1])
			return lvl
		}
		if strings.EqualFold(strings.TrimSpace(n), "title") {
			return 1
		}
	}
	if info != nil && info.outlineLvl >= 0 && info.outlineLvl < 9 {
		return info.outlineLvl + 1
	}
	return 0
}

// isTitle reports whether the style is the document Title style.
func (sm *styleMap) isTitle(styleID string) bool {
	if strings.EqualFold(styleID, "title") {
		return true
	}
	if info := sm.byID[styleID]; info != nil && strings.EqualFold(info.name, "title") {
		return true
	}
	return false
}
