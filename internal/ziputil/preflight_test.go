package ziputil

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

func TestPreflightCountsHeadersWhenAdvertisedCountIsFalse(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for i := range 3 {
		entry, err := w.Create(fmt.Sprintf("file-%d.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("body")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data := bytes.Clone(buf.Bytes())
	end := bytes.LastIndex(data, []byte("PK\x05\x06"))
	if end < 0 {
		t.Fatal("end-of-central-directory record not found")
	}
	// A parser that trusts only these fields would believe the archive has one
	// entry and then allocate all three during its central-directory walk.
	binary.LittleEndian.PutUint16(data[end+8:end+10], 1)
	binary.LittleEndian.PutUint16(data[end+10:end+12], 1)

	err := PreflightEntries(bytes.NewReader(data), int64(len(data)), 2)
	var limitErr *EntryLimitError
	if !errors.As(err, &limitErr) || limitErr.Count != 3 || limitErr.Limit != 2 {
		t.Fatalf("err = %v, want 3-entry limit error", err)
	}
}

func TestPreflightAcceptsValidArchive(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := PreflightEntries(bytes.NewReader(buf.Bytes()), int64(buf.Len()), 1); err != nil {
		t.Fatalf("PreflightEntries: %v", err)
	}
}
