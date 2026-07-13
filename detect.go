package downmark

import (
	"io"
	"mime"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"github.com/giraffesyo/downmark/internal/textenc"
)

const sniffLen = 8192

// extMIME maps extensions to MIME types for formats we care about. The
// platform mime package is unreliable for office types, so these win.
var extMIME = map[string]string{
	".docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx":     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx":     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".pdf":      "application/pdf",
	".csv":      "text/csv",
	".html":     "text/html",
	".htm":      "text/html",
	".txt":      "text/plain",
	".text":     "text/plain",
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".json":     "application/json",
	".jsonl":    "application/json",
	".xml":      "text/xml",
	".zip":      "application/zip",
}

var mimeExt = func() map[string]string {
	m := make(map[string]string, len(extMIME))
	// Iterate a fixed order so ties resolve deterministically.
	for _, ext := range []string{
		".docx", ".xlsx", ".pptx", ".pdf", ".csv", ".html", ".txt",
		".md", ".json", ".xml", ".zip",
	} {
		if _, ok := m[extMIME[ext]]; !ok {
			m[extMIME[ext]] = ext
		}
	}
	return m
}()

func mimeByExtension(ext string) string {
	if m, ok := extMIME[ext]; ok {
		return m
	}
	if m := mime.TypeByExtension(ext); m != "" {
		if parsed, _, err := mime.ParseMediaType(m); err == nil {
			return parsed
		}
	}
	return ""
}

func extensionByMIME(mimeType string) string {
	if ext, ok := mimeExt[mimeType]; ok {
		return ext
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ""
}

func isTextLike(mimeType string) bool {
	m := strings.ToLower(mimeType)
	switch {
	case strings.HasPrefix(m, "text/"),
		m == "application/json",
		m == "application/markdown",
		m == "application/xml",
		m == "application/csv",
		strings.HasPrefix(m, "application/xhtml"),
		strings.HasSuffix(m, "+json"),
		strings.HasSuffix(m, "+xml"):
		return true
	}
	return false
}

func isOOXML(mimeType string) bool {
	return strings.HasPrefix(mimeType, "application/vnd.openxmlformats-officedocument.")
}

// buildGuesses returns 1-2 ordered StreamInfo guesses combining caller hints
// with content sniffing. Hints beat sniffing. It reads a prefix of rs and
// leaves the stream at offset 0.
func buildGuesses(rs io.ReadSeeker, base StreamInfo) ([]StreamInfo, error) {
	prefix, err := readPrefix(rs)
	if err != nil {
		return nil, err
	}

	// Enhance the hint guess: fill MIME from extension or vice versa.
	enhanced := base
	enhanced.Extension = strings.ToLower(enhanced.Extension)
	if enhanced.Extension != "" && enhanced.MIMEType == "" {
		enhanced.MIMEType = mimeByExtension(enhanced.Extension)
	}
	if enhanced.MIMEType != "" && enhanced.Extension == "" {
		enhanced.Extension = extensionByMIME(enhanced.MIMEType)
	}

	det := mimetype.Detect(prefix)
	sniffedMIME := det.String()
	if parsed, _, err := mime.ParseMediaType(sniffedMIME); err == nil {
		sniffedMIME = parsed
	}
	sniffed := StreamInfo{
		MIMEType:  sniffedMIME,
		Extension: strings.ToLower(det.Extension()),
		Filename:  base.Filename,
		LocalPath: base.LocalPath,
		URL:       base.URL,
	}

	charsetFor := func(g StreamInfo) StreamInfo {
		if base.Charset != "" {
			g.Charset = strings.ToLower(base.Charset)
		} else if g.Charset == "" && isTextLike(g.MIMEType) && len(prefix) > 0 {
			g.Charset = textenc.DetectCharset(prefix)
		}
		return g
	}

	compatible := enhanced.MIMEType == "" ||
		strings.EqualFold(enhanced.MIMEType, sniffed.MIMEType) ||
		det.Is(enhanced.MIMEType) ||
		// OOXML hint sniffed as bare zip: trust the hint, zip is the container.
		(isOOXML(enhanced.MIMEType) && det.Is("application/zip")) ||
		// Any text-like hint over sniffed plain text is plausible: many text
		// formats (csv, html fragments, markdown, json) sniff as text/plain.
		(isTextLike(enhanced.MIMEType) && strings.HasPrefix(sniffed.MIMEType, "text/"))

	if compatible {
		return []StreamInfo{charsetFor(enhanced.fillFrom(sniffed))}, nil
	}
	return []StreamInfo{charsetFor(enhanced), charsetFor(sniffed)}, nil
}

func readPrefix(rs io.ReadSeeker) ([]byte, error) {
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, sniffLen)
	n, err := io.ReadFull(rs, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return buf[:n], nil
}
