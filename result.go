package downmark

// Result is the outcome of a successful conversion.
type Result struct {
	// Markdown is the normalized Markdown output.
	Markdown string
	// Title is the document title if the format provides one, else "".
	Title string
}
