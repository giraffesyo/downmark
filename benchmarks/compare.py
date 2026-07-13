#!/usr/bin/env python3
"""Run an end-to-end CLI comparison between Downmark and MarkItDown."""

import argparse
import hashlib
import statistics
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path

CORPUS = (
    "test.pdf",
    "synthetic.pdf",
    "test.docx",
    "test.xlsx",
    "test.pptx",
    "test_blog.html",
    "test_mskanji.csv",
)


@dataclass
class Result:
    milliseconds: float | None
    output_bytes: int
    digest: str
    error: str


def convert(command: str, path: Path, runs: int) -> Result:
    samples = []
    output = b""
    for attempt in range(runs + 1):  # First invocation warms files/imports.
        started = time.perf_counter()
        completed = subprocess.run([command, str(path)], capture_output=True)
        elapsed = (time.perf_counter() - started) * 1_000
        if completed.returncode:
            lines = completed.stderr.decode(errors="replace").splitlines()
            return Result(None, 0, "", lines[-1] if lines else "conversion failed")
        output = completed.stdout
        if attempt:
            samples.append(elapsed)
    return Result(statistics.median(samples), len(output), hashlib.sha256(output).hexdigest()[:12], "")


def cell(result: Result) -> str:
    if result.error:
        return f"error: {result.error}"
    return f"{result.milliseconds:.1f} ms; {result.output_bytes} B; `{result.digest}`"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--downmark", required=True, help="path to the downmark CLI")
    parser.add_argument("--markitdown", required=True, help="path to the markitdown CLI")
    parser.add_argument("--corpus-dir", type=Path, default=Path("testdata"))
    parser.add_argument("--runs", type=int, default=5, help="measured invocations per tool and fixture")
    args = parser.parse_args()

    print("| Fixture | Downmark | MarkItDown |")
    print("|---|---|---|")
    for name in CORPUS:
        path = args.corpus_dir / name
        if not path.is_file():
            raise SystemExit(f"missing corpus fixture: {path}")
        downmark = convert(args.downmark, path, args.runs)
        markitdown = convert(args.markitdown, path, args.runs)
        print(f"| {name} | {cell(downmark)} | {cell(markitdown)} |")


if __name__ == "__main__":
    main()
