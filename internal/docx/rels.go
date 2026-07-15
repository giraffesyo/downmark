package docx

import (
	"context"
	"errors"

	"github.com/giraffesyo/downmark/internal/ooxml"
)

type rel struct {
	target   string
	external bool
}

func parseRels(ctx context.Context, data []byte) (map[string]rel, error) {
	rels := map[string]rel{}
	if data == nil {
		return rels, nil
	}
	root, err := decodeTree(ctx, data)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, ooxml.ErrXMLComplexity) {
			return nil, err
		}
		return rels, nil
	}
	container := root.child("Relationships")
	if container == nil {
		return rels, nil
	}
	for _, r := range container.kids {
		if r.name != "Relationship" {
			continue
		}
		rels[r.attr("Id")] = rel{
			target:   r.attr("Target"),
			external: r.attr("TargetMode") == "External",
		}
	}
	return rels, nil
}
