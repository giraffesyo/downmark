package docx

type rel struct {
	target   string
	external bool
}

func parseRels(data []byte) map[string]rel {
	rels := map[string]rel{}
	if data == nil {
		return rels
	}
	root, err := decodeTree(data)
	if err != nil {
		return rels
	}
	container := root.child("Relationships")
	if container == nil {
		return rels
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
	return rels
}
