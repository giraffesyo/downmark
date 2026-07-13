package mdutil

import "testing"

func TestTable(t *testing.T) {
	got := Table([][]string{
		{"Name", "Age"},
		{"Alice", "30"},
		{"Bob|Bobby"},
	})
	want := "| Name | Age |\n" +
		"| --- | --- |\n" +
		"| Alice | 30 |\n" +
		"| Bob\\|Bobby |  |\n"
	if got != want {
		t.Errorf("Table = %q, want %q", got, want)
	}
}

func TestTableEmpty(t *testing.T) {
	if got := Table(nil); got != "" {
		t.Errorf("Table(nil) = %q", got)
	}
	if got := Table([][]string{{}}); got != "" {
		t.Errorf("Table(empty row) = %q", got)
	}
}

func TestEscapeCell(t *testing.T) {
	if got := EscapeCell(" a\nb|c "); got != `a b\|c` {
		t.Errorf("EscapeCell = %q", got)
	}
}
