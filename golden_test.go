package downmark_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark/all"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files with current output")

// goldenFiles lists testdata inputs whose full output is deterministic
// enough to pin byte-for-byte.
var goldenFiles = []string{
	"sample.html",
	"test_mskanji.csv",
	"test.xlsx",
	"test.zip",
	"synthetic.pdf",
	// A real generator's PDF, with four embedded Type1 fonts: the
	// synthetic fixture above exercises Form XObjects and ToUnicode, but
	// nothing here pinned what a document from the wild converts to.
	"test.pdf",
}

func TestGolden(t *testing.T) {
	if *updateGolden {
		writeSyntheticZIPFixture(t)
	}
	for _, name := range goldenFiles {
		t.Run(name, func(t *testing.T) {
			res, err := all.ConvertFile(t.Context(), filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			goldenPath := filepath.Join("testdata", "golden", name+".md")
			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(res.Markdown), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(filepath.Clean(goldenPath))
			if err != nil {
				t.Fatalf("missing golden file (run: go test -run TestGolden -update): %v", err)
			}
			if res.Markdown != string(want) {
				t.Errorf("output differs from golden %s\n---- got ----\n%s\n---- want ----\n%s",
					goldenPath, res.Markdown, want)
			}
		})
	}
}

func TestKeepDataURIs(t *testing.T) {
	path := filepath.Join("testdata", "test.docx")

	res, err := all.ConvertFile(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Markdown, "data:image/png;base64,iVBOR") {
		t.Error("default output must not embed full data URIs")
	}

	e := all.New(all.Options{KeepDataURIs: true})
	res, err = e.ConvertFile(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, "data:image/png;base64,iVBOR") {
		t.Error("KeepDataURIs output should embed the image as a data URI")
	}
}
