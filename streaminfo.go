package downmark

import (
	"slices"
	"strings"
)

// StreamInfo carries what is known or guessed about an input stream.
// The zero value means "nothing known"; every field is optional.
type StreamInfo struct {
	// MIMEType is the media type without parameters, e.g. "application/pdf".
	MIMEType string
	// Extension is lowercase with a leading dot, e.g. ".docx".
	Extension string
	// Charset is an IANA charset name, e.g. "utf-8" or "shift_jis".
	Charset string
	// Filename is the base name of the source, if known.
	Filename string
	// LocalPath is the path of the source file, if the input came from disk.
	LocalPath string
	// URL is the source URL, if the input came from the network.
	URL string
}

// Matches reports whether the stream's extension is one of exts (lowercase,
// with leading dot) or its MIME type starts with any of mimePrefixes. It is
// the standard building block for Converter.Accepts implementations.
func (s StreamInfo) Matches(exts []string, mimePrefixes []string) bool {
	if s.Extension != "" && slices.Contains(exts, strings.ToLower(s.Extension)) {
		return true
	}
	m := strings.ToLower(s.MIMEType)
	if m == "" {
		return false
	}
	for _, p := range mimePrefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

// fillFrom returns a copy of s with empty fields filled from other.
// Fields already set on s always win.
func (s StreamInfo) fillFrom(other StreamInfo) StreamInfo {
	if s.MIMEType == "" {
		s.MIMEType = other.MIMEType
	}
	if s.Extension == "" {
		s.Extension = other.Extension
	}
	if s.Charset == "" {
		s.Charset = other.Charset
	}
	if s.Filename == "" {
		s.Filename = other.Filename
	}
	if s.LocalPath == "" {
		s.LocalPath = other.LocalPath
	}
	if s.URL == "" {
		s.URL = other.URL
	}
	return s
}
