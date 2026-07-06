from __future__ import annotations

import base64
import csv
import json
import os
import shlex
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

from .config import Config
from .generation import GenerationService, parse_generated_files, write_generated_files
from .generators import GenerationError, GenerationRequest, ResponseFormat


TEST_NAMES = [
    "no_args_stdin",
    "plain_file",
    "multiple_files",
    "dash_mixed",
    "number_all",
    "number_nonblank",
    "number_nonblank_overrides_number",
    "squeeze_blank",
    "show_ends",
    "show_tabs",
    "show_nonprinting",
    "show_all",
    "option_e",
    "option_t",
    "ignored_u",
    "long_options",
    "missing_file",
]


@dataclass(frozen=True)
class Paths:
    run: Path
    specs: Path
    prompts: Path
    logs: Path
    packages: Path
    reports: Path
    bin: Path
    tests: Path
    oracle: Path


def info(message: str) -> None:
    print(message, file=sys.stderr)


def warn(message: str) -> None:
    print(f"warning: {message}", file=sys.stderr)


def die(message: str) -> None:
    print(f"error: {message}", file=sys.stderr)
    raise SystemExit(1)


def executable_exists(command: str) -> bool:
    if os.sep in command:
        return os.access(command, os.X_OK)
    return shutil.which(command) is not None


def resolve_paths(config: Config) -> Paths:
    if config.evaluate_existing is not None:
        run_dir = config.evaluate_existing
        if not run_dir.is_dir():
            die(f"existing run directory not found: {run_dir}")
    else:
        run_dir = config.run_root / config.run_id

    return Paths(
        run=run_dir,
        specs=run_dir / "specs",
        prompts=run_dir / "prompts",
        logs=run_dir / "logs",
        packages=run_dir / "packages",
        reports=run_dir / "reports",
        bin=run_dir / "bin",
        tests=run_dir / "tests",
        oracle=run_dir / "oracle",
    )


def ensure_environment(config: Config) -> None:
    if not config.source.is_file():
        die(f"source file not found: {config.source}")
    if not os.access(config.oracle, os.X_OK):
        die(f"oracle is not executable: {config.oracle}")
    commands = ["go"]
    if config.evaluate_existing is None and config.provider == "codex":
        commands.append(config.codex_bin)
    if config.evaluate_existing is None and config.provider == "opencode":
        commands.append(config.opencode_bin)
    if config.evaluate_existing is None and config.provider == "antigravity":
        commands.append(config.antigravity_bin)
    for command in commands:
        if not executable_exists(command):
            die(f"required command not found: {command}")


def create_run_dirs(paths: Paths, config: Config) -> None:
    if config.evaluate_existing is None and paths.run.exists():
        if not paths.run.is_dir() or any(paths.run.iterdir()):
            die(f"run directory already exists and is not empty: {paths.run}")
    for directory in (
        paths.specs,
        paths.prompts,
        paths.logs,
        paths.packages,
        paths.reports,
        paths.bin,
        paths.tests,
        paths.oracle,
    ):
        directory.mkdir(parents=True, exist_ok=True)
    if config.evaluate_existing is None:
        shutil.copy2(config.source, paths.run / "cat.c")


def write_bytes(path: Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


def prepare_tests(paths: Paths) -> None:
    write_bytes(paths.tests / "empty.stdin", b"")
    write_bytes(
        paths.tests / "no_args_stdin" / "stdin.txt", b"stdin line 1\nstdin line 2\n"
    )
    write_bytes(paths.tests / "plain_file" / "a.txt", b"hello\nworld\n")
    write_bytes(paths.tests / "multiple_files" / "a.txt", b"first\n")
    write_bytes(paths.tests / "multiple_files" / "b.txt", b"second\nthird\n")
    write_bytes(paths.tests / "dash_mixed" / "a.txt", b"file-a\n")
    write_bytes(paths.tests / "dash_mixed" / "stdin.txt", b"stdin-middle\n")
    write_bytes(paths.tests / "dash_mixed" / "b.txt", b"file-b\n")
    write_bytes(paths.tests / "number_all" / "a.txt", b"alpha\n\nbeta\n")
    write_bytes(paths.tests / "number_nonblank" / "a.txt", b"alpha\n\nbeta\n")
    write_bytes(
        paths.tests / "number_nonblank_overrides_number" / "a.txt", b"alpha\n\nbeta\n"
    )
    write_bytes(paths.tests / "squeeze_blank" / "a.txt", b"a\n\n\n\nb\n\n\n")
    write_bytes(paths.tests / "show_ends" / "a.txt", b"a\n\ncrlf\r\nno-newline")
    write_bytes(paths.tests / "show_tabs" / "a.txt", b"a\tb\n\tindent\n")
    write_bytes(
        paths.tests / "show_nonprinting" / "a.bin",
        b"\x01\x02\x1f\x7f\x80\xff\nplain\ttext\n",
    )
    write_bytes(paths.tests / "show_all" / "a.bin", b"a\tb\n\x01\x7f\n")
    write_bytes(paths.tests / "option_e" / "a.bin", b"x\n\x01\n")
    write_bytes(paths.tests / "option_t" / "a.bin", b"x\ty\n\x01\t\n")
    write_bytes(paths.tests / "ignored_u" / "a.txt", b"buffering flag is ignored\n")
    write_bytes(paths.tests / "long_options" / "a.txt", b"alpha\n\tbeta\n")
    write_bytes(paths.tests / "missing_file" / "a.txt", b"before\n")


def configure_test(paths: Paths, test_name: str) -> tuple[Path, list[str]]:
    test_dir = paths.tests / test_name
    stdin = paths.tests / "empty.stdin"
    args: list[str] = []

    if test_name == "no_args_stdin":
        stdin = test_dir / "stdin.txt"
    elif test_name == "plain_file":
        args = [str(test_dir / "a.txt")]
    elif test_name == "multiple_files":
        args = [str(test_dir / "a.txt"), str(test_dir / "b.txt")]
    elif test_name == "dash_mixed":
        stdin = test_dir / "stdin.txt"
        args = [str(test_dir / "a.txt"), "-", str(test_dir / "b.txt")]
    elif test_name == "number_all":
        args = ["-n", str(test_dir / "a.txt")]
    elif test_name == "number_nonblank":
        args = ["-b", str(test_dir / "a.txt")]
    elif test_name == "number_nonblank_overrides_number":
        args = ["-b", "-n", str(test_dir / "a.txt")]
    elif test_name == "squeeze_blank":
        args = ["-s", str(test_dir / "a.txt")]
    elif test_name == "show_ends":
        args = ["-E", str(test_dir / "a.txt")]
    elif test_name == "show_tabs":
        args = ["-T", str(test_dir / "a.txt")]
    elif test_name == "show_nonprinting":
        args = ["-v", str(test_dir / "a.bin")]
    elif test_name == "show_all":
        args = ["-A", str(test_dir / "a.bin")]
    elif test_name == "option_e":
        args = ["-e", str(test_dir / "a.bin")]
    elif test_name == "option_t":
        args = ["-t", str(test_dir / "a.bin")]
    elif test_name == "ignored_u":
        args = ["-u", str(test_dir / "a.txt")]
    elif test_name == "long_options":
        args = ["--number", "--show-tabs", str(test_dir / "a.txt")]
    elif test_name == "missing_file":
        args = [str(test_dir / "a.txt"), str(test_dir / "does-not-exist.txt")]
    else:
        die(f"unknown test: {test_name}")

    return stdin, args


def normalize_stderr(input_path: Path, output_path: Path) -> None:
    normalized: list[str] = []
    for line in input_path.read_text(errors="replace").splitlines():
        if ": " in line:
            normalized.append(f"<cmd>:{line.split(':', 1)[1]}")
        else:
            normalized.append(line)
    output_path.write_text("\n".join(normalized) + ("\n" if normalized else ""))


def execute_case(
    binary: Path, test_name: str, output_dir: Path, paths: Paths, timeout_seconds: int
) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    stdin_path, args = configure_test(paths, test_name)
    stdout_path = output_dir / "stdout"
    stderr_path = output_dir / "stderr"

    with (
        stdin_path.open("rb") as stdin,
        stdout_path.open("wb") as stdout,
        stderr_path.open("wb") as stderr,
    ):
        try:
            result = subprocess.run(
                [str(binary), *args],
                stdin=stdin,
                stdout=stdout,
                stderr=stderr,
                timeout=timeout_seconds,
                check=False,
            )
            status = result.returncode
        except subprocess.TimeoutExpired:
            status = 124

    (output_dir / "status").write_text(f"{status}\n")
    normalize_stderr(stderr_path, output_dir / "stderr.norm")


def generate_oracle_outputs(paths: Paths, config: Config) -> None:
    for test_name in TEST_NAMES:
        execute_case(
            config.oracle,
            test_name,
            paths.oracle / test_name,
            paths,
            config.test_timeout,
        )


def args_for_markdown(paths: Paths, test_name: str) -> str:
    _, args = configure_test(paths, test_name)
    return " ".join(shlex.quote(arg) for arg in args)


def b64(path: Path) -> str:
    return base64.b64encode(path.read_bytes()).decode("ascii")


def write_io_spec(paths: Paths, config: Config) -> None:
    lines = [
        "# Dynamic I/O Specifications",
        "",
        f"These examples were generated by executing the oracle `{config.oracle}`.",
        "A Go translation should match stdout bytes, normalized stderr, and exit status.",
        "",
    ]
    for test_name in TEST_NAMES:
        stdin_path, _ = configure_test(paths, test_name)
        oracle_dir = paths.oracle / test_name
        status = (oracle_dir / "status").read_text().strip()
        lines.extend(
            [
                f"## {test_name}",
                "",
                f"- Args: `{args_for_markdown(paths, test_name)}`",
                f"- Stdin file: `{stdin_path}`",
                f"- Expected exit status: `{status}`",
                f"- Expected stdout base64: `{b64(oracle_dir / 'stdout')}`",
                f"- Expected normalized stderr base64: `{b64(oracle_dir / 'stderr.norm')}`",
                "",
            ]
        )
    (paths.specs / "io.md").write_text("\n".join(lines))


def absolute(path: Path) -> str:
    return str(path.resolve())


def generate_specs(paths: Paths, service: GenerationService) -> None:
    source = (paths.run / "cat.c").read_text()
    static_prompt = paths.prompts / "static-spec.prompt.md"
    static_prompt.write_text(
        f"""You are the static-specification agent for SPECTRA.

Read the cat.c source embedded below and output markdown only.

Generate static specifications for translating cat.c to Go. Use this structure:
- Program input/output contract
- Option parsing contract
- Preconditions and postconditions for simple_cat behavior
- Preconditions and postconditions for formatted cat behavior
- Invariants for line numbering, squeeze blank, show tabs, show ends, and show nonprinting
- Explicit equivalences for -A, -e, -t, and ignored -u

Keep the specs precise enough to guide translation but shorter than the source code.

<cat.c>
{source}
</cat.c>
"""
    )
    (paths.specs / "static.md").write_text(
        service.generate(GenerationRequest("static-spec", static_prompt.read_text()))
    )

    descriptions_prompt = paths.prompts / "descriptions.prompt.md"
    descriptions_prompt.write_text(
        f"""You are the natural-language description agent for SPECTRA.

Read the cat.c source embedded below and output markdown only.

Write concise natural-language descriptions for the Go translator:
- What the program does at a user-visible level
- How each supported option changes output
- What state must persist across input files
- Which C-specific optimizations are not required for semantic parity in Go
- Which edge cases are most likely to be mistranslated

<cat.c>
{source}
</cat.c>
"""
    )
    (paths.specs / "descriptions.md").write_text(
        service.generate(
            GenerationRequest("descriptions", descriptions_prompt.read_text())
        )
    )


def candidate_modality_for_order(order: int) -> str:
    return ["static", "io", "descriptions"][(order - 1) % 3]


def setup_candidate_context(candidate: str, modality: str, paths: Paths) -> Path:
    package_dir = paths.packages / candidate
    context_dir = package_dir / "context"
    context_dir.mkdir(parents=True, exist_ok=True)
    shutil.copy2(paths.run / "cat.c", context_dir / "cat.c")
    if modality != "baseline":
        shutil.copy2(paths.specs / f"{modality}.md", context_dir / "spec.md")
    (context_dir / "candidate-name.txt").write_text(f"{candidate}\n")
    (context_dir / "modality.txt").write_text(f"{modality}\n")
    return package_dir


def write_candidate_prompt(package_dir: Path, group: str, modality: str) -> Path:
    source = (package_dir / "context" / "cat.c").read_text()
    prompt_file = package_dir / "TASK.md"
    if group == "baseline":
        prompt = f"""You are an isolated baseline translation agent.

Task: translate the embedded cat.c into a standalone idiomatic Go command-line program.

Output requirements:
- Return exactly one JSON object matching this shape, with no surrounding text:
  {{"files":[{{"path":"go.mod","content":"..."}},{{"path":"main.go","content":"..."}}]}}
- Use package main.
- Write go.mod and one or more .go files.
- Do not use cgo.
- Do not use third-party dependencies.
- Do not edit files under context/.
- The command should be compatible with GNU cat for the implemented behavior.
- Implement the options visible in cat.c: -A, -b, -e, -E, -n, -s, -t, -T, -u, -v and their long forms.
- The program must build with: go build .

Important: this is the baseline arm. Do not use generated SPECTRA specifications.

<cat.c>
{source}
</cat.c>
"""
    else:
        spec = (package_dir / "context" / "spec.md").read_text()
        prompt = f"""You are an isolated SPECTRA translation agent.

Task: translate the embedded cat.c into a standalone idiomatic Go command-line program using exactly one validated {modality} specification.

Output requirements:
- Return exactly one JSON object matching this shape, with no surrounding text:
  {{"files":[{{"path":"go.mod","content":"..."}},{{"path":"main.go","content":"..."}}]}}
- Use package main.
- Write go.mod and one or more .go files.
- Do not use cgo.
- Do not use third-party dependencies.
- Do not edit files under context/.
- The command should be compatible with GNU cat for the implemented behavior.
- Implement the options visible in cat.c: -A, -b, -e, -E, -n, -s, -t, -T, -u, -v and their long forms.
- Preserve state across multiple input files where cat.c does so.
- The program must build with: go build .

SPECTRA rule: use the attached {modality} spec to guide translation. Do not combine other spec modalities.

<cat.c>
{source}
</cat.c>

<spec modality="{modality}">
{spec}
</spec>
"""
    prompt_file.write_text(prompt)
    return prompt_file


def generate_candidate(
    candidate: str,
    group: str,
    modality: str,
    paths: Paths,
    service: GenerationService,
) -> None:
    package_dir = setup_candidate_context(candidate, modality, paths)
    prompt_file = write_candidate_prompt(package_dir, group, modality)
    info(f"generating candidate: {candidate} ({group}/{modality})")
    response = service.generate(
        GenerationRequest(candidate, prompt_file.read_text(), ResponseFormat.FILES_JSON)
    )
    write_generated_files(package_dir, parse_generated_files(response))


def build_candidate(candidate: str, paths: Paths) -> tuple[bool, Path]:
    package_dir = paths.packages / candidate
    eval_bin_dir = paths.run / ".eval-bin"
    eval_bin_dir.mkdir(parents=True, exist_ok=True)
    binary = eval_bin_dir.resolve() / candidate
    build_log = paths.reports / f"{candidate}.build.log"
    if not (package_dir / "go.mod").is_file():
        build_log.write_text("missing go.mod\n")
        return False, binary

    with build_log.open("wb") as log:
        tidy = subprocess.run(
            ["go", "mod", "tidy"],
            cwd=package_dir,
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
        if tidy.returncode != 0:
            return False, binary
        build = subprocess.run(
            ["go", "build", "-o", str(binary), "."],
            cwd=package_dir,
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
    return build.returncode == 0 and binary.is_file() and os.access(
        binary, os.X_OK
    ), binary


def append_tsv(path: Path, row: list[object]) -> None:
    with path.open("a", newline="") as file:
        writer = csv.writer(file, delimiter="\t", lineterminator="\n")
        writer.writerow(row)


def score_candidate(
    candidate: str,
    group: str,
    group_order: int,
    modality: str,
    paths: Paths,
    config: Config,
) -> None:
    total = len(TEST_NAMES)
    built, binary = build_candidate(candidate, paths)
    if not built:
        append_tsv(
            paths.reports / "scores.tsv",
            [
                group,
                group_order,
                candidate,
                modality,
                "build_failed",
                0,
                total,
                "0.000000",
                0,
            ],
        )
        return

    passed = 0
    for test_name in TEST_NAMES:
        candidate_output = paths.reports / "candidate-output" / candidate / test_name
        expected_output = paths.oracle / test_name
        execute_case(binary, test_name, candidate_output, paths, config.test_timeout)

        actual_status = (candidate_output / "status").read_text().strip()
        expected_status = (expected_output / "status").read_text().strip()
        stdout_match = (candidate_output / "stdout").read_bytes() == (
            expected_output / "stdout"
        ).read_bytes()
        stderr_match = (candidate_output / "stderr.norm").read_bytes() == (
            expected_output / "stderr.norm"
        ).read_bytes()
        status_match = actual_status == expected_status
        test_pass = stdout_match and stderr_match and status_match
        if test_pass:
            passed += 1
        append_tsv(
            paths.reports / "test-results.tsv",
            [
                candidate,
                test_name,
                int(test_pass),
                int(stdout_match),
                int(stderr_match),
                int(status_match),
                expected_status,
                actual_status,
                candidate_output,
            ],
        )

    score = passed / total
    append_tsv(
        paths.reports / "scores.tsv",
        [
            group,
            group_order,
            candidate,
            modality,
            "built",
            passed,
            total,
            f"{score:.6f}",
            int(passed == total),
        ],
    )


def generate_all_candidates(
    paths: Paths, config: Config, service: GenerationService
) -> None:
    for i in range(1, config.candidates + 1):
        candidate = f"baseline_{i}"
        try:
            generate_candidate(candidate, "baseline", "baseline", paths, service)
        except GenerationError as exc:
            warn(f"candidate generation failed for {candidate}: {exc}")
    for i in range(1, config.candidates + 1):
        modality = candidate_modality_for_order(i)
        round_number = ((i - 1) // 3) + 1
        candidate = f"spectra_{modality}_{round_number}"
        try:
            generate_candidate(candidate, "spectra", modality, paths, service)
        except GenerationError as exc:
            warn(f"candidate generation failed for {candidate}: {exc}")


def evaluate_all_candidates(paths: Paths, config: Config) -> None:
    with (paths.reports / "scores.tsv").open("w", newline="") as file:
        csv.writer(file, delimiter="\t", lineterminator="\n").writerow(
            [
                "group",
                "group_order",
                "candidate",
                "modality",
                "build_status",
                "passed",
                "total",
                "score",
                "full_pass",
            ]
        )
    with (paths.reports / "test-results.tsv").open("w", newline="") as file:
        csv.writer(file, delimiter="\t", lineterminator="\n").writerow(
            [
                "candidate",
                "test",
                "pass",
                "stdout_match",
                "stderr_match",
                "status_match",
                "expected_status",
                "actual_status",
                "output_dir",
            ]
        )

    for i in range(1, config.candidates + 1):
        score_candidate(f"baseline_{i}", "baseline", i, "baseline", paths, config)
    for i in range(1, config.candidates + 1):
        modality = candidate_modality_for_order(i)
        round_number = ((i - 1) // 3) + 1
        score_candidate(
            f"spectra_{modality}_{round_number}", "spectra", i, modality, paths, config
        )


def read_scores(paths: Paths) -> list[dict[str, str]]:
    with (paths.reports / "scores.tsv").open(newline="") as file:
        return list(csv.DictReader(file, delimiter="\t"))


def best_score_at_k(rows: list[dict[str, str]], group: str, k: int) -> float:
    scores = [
        float(row["score"])
        for row in rows
        if row["group"] == group and int(row["group_order"]) <= k
    ]
    return max(scores, default=0.0)


def any_full_pass_at_k(rows: list[dict[str, str]], group: str, k: int) -> int:
    return int(
        any(
            row["group"] == group
            and int(row["group_order"]) <= k
            and row["full_pass"] == "1"
            for row in rows
        )
    )


def write_summary(paths: Paths, config: Config) -> None:
    rows = read_scores(paths)
    provider = config.provider or "not recorded"
    model = config.model or "not recorded"
    manifest_path = paths.run / "run.json"
    if manifest_path.is_file() and config.evaluate_existing is not None:
        try:
            manifest = json.loads(manifest_path.read_text())
        except json.JSONDecodeError:
            warn(f"ignoring invalid run manifest: {manifest_path}")
        else:
            provider = manifest.get("provider") or provider
            model = manifest.get("model") or model
    lines = [
        "# SPECTRA cat.c to Go Run",
        "",
        f"- Run id: `{paths.run.name}`",
        f"- Provider: `{provider}`",
        f"- Model: `{model}`",
        f"- Oracle: `{config.oracle}`",
        f"- Source: `{config.source}`",
        f"- Candidates per group: `{config.candidates}`",
        f"- Tests: `{len(TEST_NAMES)}`",
        "",
        "## Improvement Over Baseline",
        "",
        "| k | baseline best@k | spectra best@k | absolute improvement | relative improvement | baseline pass@k | spectra pass@k |",
        "|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for k in range(1, config.candidates + 1):
        baseline_best = best_score_at_k(rows, "baseline", k)
        spectra_best = best_score_at_k(rows, "spectra", k)
        absolute = spectra_best - baseline_best
        if baseline_best == 0:
            relative = "0.000000" if spectra_best == 0 else "inf"
        else:
            relative = f"{((spectra_best - baseline_best) / baseline_best):.6f}"
        lines.append(
            f"| {k} | {baseline_best:.6f} | {spectra_best:.6f} | {absolute:.6f} | {relative} | "
            f"{any_full_pass_at_k(rows, 'baseline', k)} | {any_full_pass_at_k(rows, 'spectra', k)} |"
        )

    lines.extend(
        [
            "",
            "## Candidate Scores",
            "",
            "See `scores.tsv` for machine-readable results.",
            "",
        ]
    )
    for row in rows:
        lines.append(
            f"- `{row['candidate']}`: group={row['group']} modality={row['modality']} build={row['build_status']} "
            f"passed={row['passed']}/{row['total']} score={row['score']} full_pass={row['full_pass']}"
        )
    lines.extend(
        [
            "",
            "## Scoring Definition",
            "",
            "- `score = passed_tests / total_tests`",
            "- `best@k = max(score)` among candidates in that group with order <= k",
            "- `absolute improvement = spectra best@k - baseline best@k`",
            "- `relative improvement = (spectra best@k - baseline best@k) / baseline best@k`",
            "- `pass@k = 1` if any candidate in that group with order <= k passes every test",
        ]
    )
    (paths.reports / "summary.md").write_text("\n".join(lines) + "\n")


def run(config: Config, service: GenerationService | None = None) -> int:
    ensure_environment(config)
    paths = resolve_paths(config)
    create_run_dirs(paths, config)

    info(f"run directory: {paths.run}")
    if config.evaluate_existing is None:
        info(f"provider/model: {config.provider}/{config.model}")

    prepare_tests(paths)
    generate_oracle_outputs(paths, config)

    if config.evaluate_existing is not None:
        info("evaluating existing generated packages only")
        evaluate_all_candidates(paths, config)
        write_summary(paths, config)
        info(f"summary: {paths.reports / 'summary.md'}")
        info(f"scores: {paths.reports / 'scores.tsv'}")
        return 0

    write_io_spec(paths, config)
    if service is None:
        raise RuntimeError("generation service is required for a generation run")
    service.write_manifest()
    generate_specs(paths, service)
    generate_all_candidates(paths, config, service)
    evaluate_all_candidates(paths, config)
    write_summary(paths, config)

    info(f"summary: {paths.reports / 'summary.md'}")
    info(f"scores: {paths.reports / 'scores.tsv'}")
    return 0
