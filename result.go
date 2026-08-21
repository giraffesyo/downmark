package downmark

// Result is the outcome of a successful conversion.
type Result struct {
	// Markdown is the normalized Markdown output.
	Markdown string
	// Title is the document title if the format provides one, else "".
	Title string
	// Warnings reports what the conversion lost, and is nil when it lost
	// nothing. A Result carrying warnings is still a success: use them to
	// decide whether the Markdown is complete enough for the job, not
	// whether the conversion worked. Bounded by MaxWarnings.
	Warnings []Warning
}
