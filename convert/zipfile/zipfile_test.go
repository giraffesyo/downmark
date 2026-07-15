package zipfile

import (
	stdzip "archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/convert/csv"
	"github.com/giraffesyo/downmark/convert/docx"
	"github.com/giraffesyo/downmark/internal/ooxml"
)

type testMember struct {
	name string
	data []byte
}

func buildArchive(t testing.TB, members ...testMember) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := stdzip.NewWriter(&buf)
	for _, member := range members {
		entry, err := w.Create(member.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(member.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var benchmarkMember []byte

func BenchmarkReadMember(b *testing.B) {
	payload := bytes.Repeat([]byte("compressible benchmark payload\n"), 32*1024)
	data := buildArchive(b, testMember{name: "member.txt", data: payload})
	zr, err := stdzip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		member, _, err := readMember(b.Context(), zr.File[0], maxTotalMemberBytes)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkMember = member
	}
}

func archiveEngine() *downmark.Engine {
	e := downmark.New()
	csv.Register(e)
	docx.Register(e, docx.Options{})
	Register(e)
	return e
}

func convertArchive(t *testing.T, e *downmark.Engine, data []byte) (*downmark.Result, error) {
	t.Helper()
	return convertArchiveContext(t.Context(), t, e, data)
}

func convertArchiveContext(ctx context.Context, t *testing.T, e *downmark.Engine, data []byte) (*downmark.Result, error) {
	t.Helper()
	return e.Convert(ctx, bytes.NewReader(data), downmark.StreamInfo{Extension: ".zip"})
}

func TestMixedArchive(t *testing.T) {
	docxData, err := os.ReadFile(filepath.Join("..", "..", "testdata", "test.docx"))
	if err != nil {
		t.Fatal(err)
	}
	data := buildArchive(t,
		testMember{name: "notes.txt", data: []byte("meeting notes body")},
		testMember{name: "data/report.csv", data: []byte("col_a,col_b\n1,2\n")},
		testMember{name: "resume.docx", data: docxData},
		testMember{name: "image.png", data: []byte("\x89PNG\r\n\x1a\n")},
	)

	res, err := convertArchive(t, archiveEngine(), data)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	for _, want := range []string{
		"## File: notes.txt",
		"meeting notes body",
		"## File: data/report.csv",
		"| col_a | col_b |",
		"## File: resume.docx",
		"# Abstract",
	} {
		if !strings.Contains(res.Markdown, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(res.Markdown, "image.png") {
		t.Errorf("unsupported image should be skipped silently:\n%s", res.Markdown)
	}
}

func TestSkipRules(t *testing.T) {
	data := buildArchive(t,
		testMember{name: "folder/"},
		testMember{name: "__MACOSX/notes.txt", data: []byte("resource fork junk")},
		testMember{name: ".hidden.txt", data: []byte("root dotfile junk")},
		testMember{name: "folder/.hidden.txt", data: []byte("nested dotfile junk")},
		testMember{name: "inner.ZIP", data: []byte("nested archive junk")},
		testMember{name: "visible.txt", data: []byte("visible body")},
	)

	res, err := convertArchive(t, archiveEngine(), data)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "## File: visible.txt\n\nvisible body") {
		t.Errorf("visible member missing:\n%s", res.Markdown)
	}
	for _, unwanted := range []string{"resource fork junk", "dotfile junk", "nested archive junk"} {
		if strings.Contains(res.Markdown, unwanted) {
			t.Errorf("output contains skipped content %q", unwanted)
		}
	}
}

type failingMemberConverter struct{}

func (failingMemberConverter) Name() string { return "member-failure" }
func (failingMemberConverter) Accepts(info downmark.StreamInfo) bool {
	return info.Extension == ".fail"
}
func (failingMemberConverter) Convert(context.Context, io.ReadSeeker, downmark.StreamInfo) (*downmark.Result, error) {
	return nil, errors.New("intentional member failure")
}

func TestConversionFailureIsVisible(t *testing.T) {
	e := archiveEngine()
	e.Register(failingMemberConverter{}, downmark.PrioritySpecific)
	data := buildArchive(t, testMember{name: "broken.fail", data: []byte{0, 1, 2, 3}})

	res, err := convertArchive(t, e, data)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "## File: broken.fail") ||
		!strings.Contains(res.Markdown, "[conversion failed:") ||
		!strings.Contains(res.Markdown, "intentional member failure") {
		t.Errorf("failure note missing:\n%s", res.Markdown)
	}
}

func TestMemberSizeLimit(t *testing.T) {
	oversized := make([]byte, maxMemberBytes+1)
	data := buildArchive(t,
		testMember{name: "large.txt", data: oversized},
		testMember{name: "small.txt", data: []byte("small body")},
	)

	res, err := convertArchive(t, archiveEngine(), data)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "[skipped: member exceeds 10MB uncompressed]") {
		t.Errorf("size-limit note missing:\n%s", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "small body") {
		t.Error("conversion did not continue after oversized member")
	}
}

func TestNestedOfficePartLimit(t *testing.T) {
	docxData, err := os.ReadFile(filepath.Join("..", "..", "testdata", "test.docx"))
	if err != nil {
		t.Fatal(err)
	}
	docxData = setCentralUncompressedSize(t, docxData, "word/document.xml", ooxml.MaxPartBytes+1)
	data := buildArchive(t, testMember{name: "oversized.docx", data: docxData})

	res, err := convertArchive(t, archiveEngine(), data)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	for _, want := range []string{
		"## File: oversized.docx",
		"[conversion failed:",
		"word/document.xml",
		"exceeds 16777216-byte limit",
	} {
		if !strings.Contains(res.Markdown, want) {
			t.Errorf("output missing %q:\n%s", want, res.Markdown)
		}
	}
}

func setCentralUncompressedSize(t *testing.T, data []byte, name string, size uint32) []byte {
	t.Helper()
	data = bytes.Clone(data)
	for offset := 0; ; {
		index := bytes.Index(data[offset:], []byte("PK\x01\x02"))
		if index < 0 {
			break
		}
		offset += index
		if offset+46 > len(data) {
			break
		}
		nameLen := int(binary.LittleEndian.Uint16(data[offset+28 : offset+30]))
		extraLen := int(binary.LittleEndian.Uint16(data[offset+30 : offset+32]))
		commentLen := int(binary.LittleEndian.Uint16(data[offset+32 : offset+34]))
		end := offset + 46 + nameLen + extraLen + commentLen
		if end > len(data) {
			break
		}
		if string(data[offset+46:offset+46+nameLen]) == name {
			binary.LittleEndian.PutUint32(data[offset+24:offset+28], size)
			return data
		}
		offset = end
	}
	t.Fatalf("central-directory entry %q not found", name)
	return nil
}

func TestConvertedMemberLimit(t *testing.T) {
	members := make([]testMember, 0, maxConvertedMembers+1)
	for i := range maxConvertedMembers + 1 {
		members = append(members, testMember{
			name: fmt.Sprintf("member-%02d.txt", i),
			data: []byte("body"),
		})
	}
	res, err := convertArchive(t, archiveEngine(), buildArchive(t, members...))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if got := strings.Count(res.Markdown, "## File:"); got != maxConvertedMembers {
		t.Errorf("converted headings = %d, want %d", got, maxConvertedMembers)
	}
	if !strings.Contains(res.Markdown, "[... additional archive members skipped ...]") {
		t.Error("member-limit note missing")
	}
}

func TestUnsupportedMembersDoNotConsumeConvertedLimit(t *testing.T) {
	members := make([]testMember, 0, maxConvertedMembers+1)
	for i := range maxConvertedMembers {
		members = append(members, testMember{
			name: fmt.Sprintf("image-%02d.png", i),
			data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
		})
	}
	members = append(members, testMember{name: "last.txt", data: []byte("last convertible member")})

	res, err := convertArchive(t, archiveEngine(), buildArchive(t, members...))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(res.Markdown, "## File: last.txt\n\nlast convertible member") {
		t.Errorf("valid member after unsupported files was skipped:\n%s", res.Markdown)
	}
	if strings.Contains(res.Markdown, "additional archive members skipped") {
		t.Error("unsupported members must not consume the converted-member budget")
	}
}

func TestArchiveEntryLimit(t *testing.T) {
	members := make([]testMember, 0, maxArchiveEntries+1)
	for i := range maxArchiveEntries + 1 {
		members = append(members, testMember{name: fmt.Sprintf("directory-%04d/", i)})
	}

	_, err := convertArchive(t, archiveEngine(), buildArchive(t, members...))
	want := fmt.Sprintf("archive contains %d entries; limit is %d", maxArchiveEntries+1, maxArchiveEntries)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want archive-entry-limit error", err)
	}
}

type zeroReaderAt struct{}

func (zeroReaderAt) ReadAt(p []byte, _ int64) (int, error) {
	clear(p)
	return len(p), nil
}

func TestCompressedArchiveSizeLimit(t *testing.T) {
	input := io.NewSectionReader(zeroReaderAt{}, 0, maxArchiveBytes+1)
	_, err := New(archiveEngine()).Convert(t.Context(), input, downmark.StreamInfo{Extension: ".zip"})
	want := fmt.Sprintf("archive is %d bytes; limit is %d", maxArchiveBytes+1, maxArchiveBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want compressed-archive-limit error", err)
	}
}

func TestRemainingMemberBudgetIsCheckedBeforeRead(t *testing.T) {
	data := buildArchive(t, testMember{name: "member.txt", data: []byte("body")})
	zr, err := stdzip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	_, readBytes, err := readMember(t.Context(), zr.File[0], int64(len("body")-1))
	if !errors.Is(err, errMemberBudget) || readBytes != 0 {
		t.Fatalf("err = %v, readBytes = %d; want member-budget error before reading", err, readBytes)
	}
}

type cancelingMemberConverter struct {
	cancel context.CancelFunc
}

func (cancelingMemberConverter) Accepts(info downmark.StreamInfo) bool {
	return info.Extension == ".cancel"
}
func (c cancelingMemberConverter) Convert(context.Context, io.ReadSeeker, downmark.StreamInfo) (*downmark.Result, error) {
	c.cancel()
	return nil, context.Canceled
}

func TestMemberCancellationIsPropagated(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	e := archiveEngine()
	e.Register(cancelingMemberConverter{cancel: cancel}, downmark.PrioritySpecific)

	res, err := convertArchiveContext(ctx, t, e, buildArchive(t, testMember{
		name: "last.cancel",
		data: []byte("cancel during the final member"),
	}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v (res=%v), want context.Canceled", err, res)
	}
}

type expandingConverter struct{}

func (expandingConverter) Accepts(info downmark.StreamInfo) bool { return info.Extension == ".expand" }
func (expandingConverter) Convert(context.Context, io.ReadSeeker, downmark.StreamInfo) (*downmark.Result, error) {
	return &downmark.Result{Markdown: strings.Repeat("x", maxOutputBytes)}, nil
}

func TestOutputLimit(t *testing.T) {
	e := archiveEngine()
	e.Register(expandingConverter{}, downmark.PrioritySpecific)
	_, err := convertArchive(t, e, buildArchive(t, testMember{name: "large.expand", data: []byte("x")}))
	want := fmt.Sprintf("zip: output exceeds %d-byte limit", maxOutputBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want output-limit error", err)
	}
}

func TestEncryptedArchive(t *testing.T) {
	data := markEncrypted(t, buildArchive(t, testMember{name: "secret.txt", data: []byte("secret")}))
	_, err := convertArchive(t, archiveEngine(), data)
	if !errors.Is(err, errEncryptedArchive) {
		t.Fatalf("err = %v, want encrypted-archive error", err)
	}
}

func markEncrypted(t *testing.T, data []byte) []byte {
	t.Helper()
	data = bytes.Clone(data)
	local := bytes.Index(data, []byte("PK\x03\x04"))
	central := bytes.Index(data, []byte("PK\x01\x02"))
	if local < 0 || central < 0 {
		t.Fatal("archive headers not found")
	}
	binary.LittleEndian.PutUint16(data[local+6:], binary.LittleEndian.Uint16(data[local+6:])|zipFlagEncrypted)
	binary.LittleEndian.PutUint16(data[central+8:], binary.LittleEndian.Uint16(data[central+8:])|zipFlagEncrypted)
	return data
}

func TestNoConvertibleMembers(t *testing.T) {
	data := buildArchive(t, testMember{
		name: "image.png",
		data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
	})
	_, err := convertArchive(t, archiveEngine(), data)
	if !errors.Is(err, errNoConvertible) {
		t.Fatalf("err = %v, want no-convertible-files error", err)
	}
}
