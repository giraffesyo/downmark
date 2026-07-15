# Downmark comparison benchmarks

This directory compares Downmark and MarkItDown as document-to-Markdown
converters. It intentionally measures their CLI interfaces over the same
fixtures, including process startup and output serialization.

The runner reports successful conversion, median wall time, output size, and
an output digest. It alternates which CLI runs first on each measured pair to
reduce order bias. A digest makes a changed output visible without treating
different-but-valid Markdown renderings as failures. Downmark's own golden
tests remain the correctness contract for its output.

## Reproduce

From the repository root:

```sh
go build -o /tmp/downmark ./cmd/downmark
uv venv /tmp/downmark-markitdown
uv pip install --python /tmp/downmark-markitdown/bin/python \
  'markitdown[pdf,docx,xlsx,pptx]==0.1.6'
python benchmarks/compare.py \
  --downmark /tmp/downmark \
  --markitdown /tmp/downmark-markitdown/bin/markitdown \
  --runs 10
```

The fixture corpus covers PDF (including a synthetic Form XObject and
ToUnicode case), DOCX, XLSX, PPTX, HTML, and Shift-JIS CSV. Results must
include the Downmark commit, MarkItDown/Python version, OS, CPU, and the
commands above. Re-run on the target environment before using timings for a
performance decision.
