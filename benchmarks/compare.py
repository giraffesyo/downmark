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


def convert_pair(
    commands: tuple[str, str], path: Path, runs: int
) -> tuple[Result, Result]:
    samples: tuple[list[float], list[float]] = ([], [])
    outputs = [b"", b""]
    errors = ["", ""]
    for attempt in range(runs + 1):  # First invocation warms files/imports.
        order = (0, 1) if attempt % 2 == 0 else (1, 0)
        for index in order:
            if errors[index]:
                continue
            started = time.perf_counter()
            completed = subprocess.run([commands[index], str(path)], capture_output=True)
            elapsed = (time.perf_counter() - started) * 1_000
            if completed.returncode:
                lines = completed.stderr.decode(errors="replace").splitlines()
                errors[index] = lines[-1] if lines else "conversion failed"
                continue
            outputs[index] = completed.stdout
            if attempt:
                samples[index].append(elapsed)

    results = []
    for index in range(2):
        if errors[index]:
            results.append(Result(None, 0, "", errors[index]))
            continue
        output = outputs[index]
        results.append(
            Result(
                statistics.median(samples[index]),
                len(output),
                hashlib.sha256(output).hexdigest()[:12],
                "",
            )
        )
    return results[0], results[1]


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
        downmark, markitdown = convert_pair(
            (args.downmark, args.markitdown), path, args.runs
        )
        print(f"| {name} | {cell(downmark)} | {cell(markitdown)} |")


if __name__ == "__main__":
    main()
