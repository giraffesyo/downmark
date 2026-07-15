// Package zipfile walks ZIP archives and converts supported members to
// Markdown through the downmark engine that registered it.
package zipfile

import (
	stdzip "archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/internal/ctxio"
	"github.com/giraffesyo/downmark/internal/readerat"
	"github.com/giraffesyo/downmark/internal/ziputil"
)

const (
	maxArchiveEntries          = 1024
	maxArchiveBytes            = 64 * 1024 * 1024
	maxConvertedMembers        = 64
	maxMemberBytes             = 10 * 1024 * 1024
	maxTotalMemberBytes        = 128 * 1024 * 1024
	maxOutputBytes             = 32 * 1024 * 1024
	zipFlagEncrypted    uint16 = 1 << 0
)

var (
	errEncryptedArchive = errors.New("zip: encrypted archives are not supported")
	errNestedArchive    = errors.New("zip: nested archives are not converted")
	errNoConvertible    = errors.New("zip contained no convertible files")
	errMemberBudget     = errors.New("zip: total uncompressed member data exceeds limit")
)

// New returns a ZIP converter that delegates archive members back to e.
func New(e *downmark.Engine) downmark.Converter { return converter{engine: e} }

// Register adds the ZIP converter to e after specific-format converters and
// before generic converters in priority order.
func Register(e *downmark.Engine) { e.Register(New(e), downmark.PriorityArchive) }

type converter struct {
	engine *downmark.Engine
}

func (converter) Name() string { return "zip" }

func (converter) InputLimit() int64 { return maxArchiveBytes }

func (converter) Accepts(info downmark.StreamInfo) bool {
	return info.Matches(
		[]string{".zip"},
		[]string{"application/zip", "application/x-zip", "application/x-zip-compressed"},
	)
}

type archiveMemberContextKey struct{}

func (c converter) Convert(ctx context.Context, input io.ReadSeeker, _ downmark.StreamInfo) (*downmark.Result, error) {
	if ctx.Value(archiveMemberContextKey{}) != nil {
		return nil, errNestedArchive
	}
	if c.engine == nil {
		return nil, errors.New("zip: converter has no engine")
	}

	ra, size, err := readerat.FromLimit(input, maxArchiveBytes)
	if errors.Is(err, readerat.ErrTooLarge) {
		return nil, fmt.Errorf("%w: zip archive is %d bytes; limit is %d", downmark.ErrInputTooLarge, size, maxArchiveBytes)
	}
	if err != nil {
		return nil, fmt.Errorf("zip: reading archive: %w", err)
	}
	if err := ziputil.PreflightEntries(ra, size, maxArchiveEntries); err != nil {
		return nil, err
	}
	zr, err := stdzip.NewReader(ra, size)
	if err != nil {
		return nil, fmt.Errorf("zip: opening archive: %w", err)
	}
	if len(zr.File) > maxArchiveEntries {
		return nil, fmt.Errorf("zip: archive contains %d entries; limit is %d", len(zr.File), maxArchiveEntries)
	}
	for _, file := range zr.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.Flags&zipFlagEncrypted != 0 {
			return nil, errEncryptedArchive
		}
	}

	memberCtx := context.WithValue(ctx, archiveMemberContextKey{}, true)
	outputLimit := maxOutputBytes
	if limit, ok := downmark.ResultLimit(ctx); ok {
		outputLimit = min(outputLimit, limit)
	}
	var out strings.Builder
	converted := 0
	var memberBytes int64
	for _, file := range zr.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if skipMember(file) {
			continue
		}
		if converted >= maxConvertedMembers {
			if err := appendLimited(&out, "\n[... additional archive members skipped ...]\n", outputLimit); err != nil {
				return nil, err
			}
			break
		}

		member, readBytes, err := readMember(ctx, file, maxTotalMemberBytes-memberBytes)
		memberBytes += readBytes
		if ctxErr := contextError(ctx, err); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, errMemberBudget) {
			return nil, fmt.Errorf("%w (%dMB)", errMemberBudget, maxTotalMemberBytes/(1024*1024))
		}
		if err != nil {
			if err := appendSection(&out, file.Name, fmt.Sprintf("[skipped: %v]", err), outputLimit); err != nil {
				return nil, err
			}
			converted++
			continue
		}

		base := path.Base(file.Name)
		info := downmark.StreamInfo{
			Extension: strings.ToLower(path.Ext(base)),
			Filename:  base,
		}
		resultLimit, err := sectionResultLimit(&out, file.Name, outputLimit)
		if err != nil {
			return nil, err
		}
		res, err := c.engine.Convert(downmark.WithResultLimit(memberCtx, resultLimit), bytes.NewReader(member), info)
		if ctxErr := contextError(ctx, err); ctxErr != nil {
			return nil, ctxErr
		}
		if err != nil {
			switch {
			case errors.Is(err, downmark.ErrUnsupportedFormat), errors.Is(err, errNestedArchive):
				continue
			case errors.Is(err, downmark.ErrResultTooLarge):
				return nil, outputLimitError(outputLimit)
			default:
				if err := appendSection(&out, file.Name, fmt.Sprintf("[conversion failed: %v]", err), outputLimit); err != nil {
					return nil, err
				}
				converted++
				continue
			}
		}
		if err := appendSection(&out, file.Name, res.Markdown, outputLimit); err != nil {
			return nil, err
		}
		converted++
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if out.Len() == 0 {
		return nil, errNoConvertible
	}
	return &downmark.Result{Markdown: out.String()}, nil
}

func skipMember(file *stdzip.File) bool {
	base := path.Base(file.Name)
	return file.FileInfo().IsDir() ||
		strings.HasPrefix(file.Name, "__MACOSX/") ||
		strings.HasPrefix(base, ".") ||
		strings.EqualFold(path.Ext(base), ".zip")
}

func readMember(ctx context.Context, file *stdzip.File, remaining int64) ([]byte, int64, error) {
	if file.UncompressedSize64 > maxMemberBytes {
		return nil, 0, fmt.Errorf("member exceeds %dMB uncompressed", maxMemberBytes/(1024*1024))
	}
	if remaining <= 0 || file.UncompressedSize64 > uint64(remaining) {
		return nil, 0, errMemberBudget
	}
	r, err := file.Open()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = r.Close() }() // read-only member

	readLimit := min(int64(maxMemberBytes), remaining)
	var member bytes.Buffer
	member.Grow(int(file.UncompressedSize64) + 1)
	_, err = member.ReadFrom(io.LimitReader(ctxio.NewReader(ctx, r), readLimit+1))
	readBytes := int64(member.Len())
	if readBytes > remaining {
		return nil, readBytes, errMemberBudget
	}
	if readBytes > maxMemberBytes {
		return nil, readBytes, fmt.Errorf("member exceeds %dMB uncompressed", maxMemberBytes/(1024*1024))
	}
	if err != nil {
		return nil, readBytes, err
	}
	return member.Bytes(), readBytes, nil
}

func contextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func appendSection(out *strings.Builder, name, markdown string, limit int) error {
	const heading = "\n## File: "
	if err := reserveOutput(out, limit, len(heading), len(name), 2, len(markdown), 1); err != nil {
		return err
	}
	out.WriteString(heading)
	out.WriteString(name)
	out.WriteString("\n\n")
	out.WriteString(markdown)
	out.WriteByte('\n')
	return nil
}

func appendLimited(out *strings.Builder, text string, limit int) error {
	if err := reserveOutput(out, limit, len(text)); err != nil {
		return err
	}
	out.WriteString(text)
	return nil
}

func sectionResultLimit(out *strings.Builder, name string, limit int) (int, error) {
	const overhead = len("\n## File: ") + 2 + 1
	remaining := limit - out.Len() - overhead - len(name)
	if remaining <= 0 {
		return 0, outputLimitError(limit)
	}
	return remaining, nil
}

func reserveOutput(out *strings.Builder, limit int, sizes ...int) error {
	remaining := limit - out.Len()
	for _, size := range sizes {
		if size > remaining {
			return outputLimitError(limit)
		}
		remaining -= size
	}
	return nil
}

func outputLimitError(limit int) error {
	return fmt.Errorf("zip: output exceeds %d-byte limit", limit)
}
