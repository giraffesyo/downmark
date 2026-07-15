package readerat

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type readSeekerOnly struct {
	reader *strings.Reader
	reads  int
}

func (r *readSeekerOnly) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

func (r *readSeekerOnly) Seek(offset int64, whence int) (int64, error) {
	return r.reader.Seek(offset, whence)
}

func TestFromLimitRejectsBeforeBuffering(t *testing.T) {
	r := &readSeekerOnly{reader: strings.NewReader("oversized")}
	ra, size, err := FromLimit(r, 4)
	if !errors.Is(err, ErrTooLarge) || ra != nil || size != int64(len("oversized")) {
		t.Fatalf("FromLimit = (%v, %d, %v), want (nil, %d, ErrTooLarge)", ra, size, err, len("oversized"))
	}
	if r.reads != 0 {
		t.Errorf("Read called %d times; oversized stream should not be buffered", r.reads)
	}
}

func TestFromLimitBuffersWithinLimit(t *testing.T) {
	r := &readSeekerOnly{reader: strings.NewReader("content")}
	ra, size, err := FromLimit(r, 16)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, size)
	if _, err := ra.ReadAt(data, 0); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Errorf("data = %q", data)
	}
}
