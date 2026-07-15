package csv

import (
	"errors"
	"strings"
	"testing"

	"github.com/giraffesyo/downmark"
)

func TestResultLimitStopsTableRendering(t *testing.T) {
	e := downmark.New(downmark.WithoutBuiltins())
	Register(e)
	input := "a,b,c,d,e,f,g,h\n1,2,3,4,5,6,7,8\n"
	res, err := e.Convert(
		downmark.WithResultLimit(t.Context(), 32),
		strings.NewReader(input),
		downmark.StreamInfo{Extension: ".csv"},
	)
	if !errors.Is(err, downmark.ErrResultTooLarge) {
		t.Fatalf("err = %v (res=%v), want ErrResultTooLarge", err, res)
	}
}
