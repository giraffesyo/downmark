package limitbuf

import (
	"errors"
	"testing"
)

func TestBufferRejectsWriteWithoutPartialOutput(t *testing.T) {
	b := New(4)
	if _, err := b.WriteString("abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteString("de"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if got := b.String(); got != "abc" {
		t.Fatalf("String = %q, want %q", got, "abc")
	}
	if err := b.WriteByte('x'); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("sticky err = %v, want ErrTooLarge", err)
	}
}
