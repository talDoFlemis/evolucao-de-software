from __future__ import annotations

import argparse
import os
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path


PROVIDERS = ("openrouter", "codex", "opencode", "antigravity")


@dataclass(frozen=True)
class Config:
    provider: str | None
    model: str | None
    candidates: int
    run_id: str
    run_root: Path
    source: Path
    oracle: Path
    test_timeout: int
    generation_timeout: int
    max_retries: int
    evaluate_existing: Path | None
    codex_bin: str
    opencode_bin: str
    antigravity_bin: str
    log_level: str


def utc_run_id() -> str:
    return datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")


def positive_int(value: str) -> int:
    parsed = int(value, 10)
    if parsed <= 0:
        raise argparse.ArgumentTypeError(f"must be a positive integer: {value}")
    return parsed


def non_negative_int(value: str) -> int:
    parsed = int(value, 10)
    if parsed < 0:
        raise argparse.ArgumentTypeError(f"must be a non-negative integer: {value}")
    return parsed


def parse_args(argv: list[str] | None = None) -> Config:
    parser = argparse.ArgumentParser(
        description="Run a provider-neutral SPECTRA C-to-Go translation workflow."
    )
    parser.add_argument("--provider", choices=PROVIDERS)
    parser.add_argument("--model", help="explicit model identifier")
    parser.add_argument("--candidates", type=positive_int, default=3)
    parser.add_argument("--run-id", default=utc_run_id())
    parser.add_argument("--run-root", type=Path, default=Path(".spectra-runs"))
    parser.add_argument("--source", type=Path, default=Path("cat.c"))
    parser.add_argument("--oracle", type=Path, default=Path("/usr/bin/cat"))
    parser.add_argument(
        "--timeout", type=positive_int, default=5, help="per-test timeout in seconds"
    )
    parser.add_argument(
        "--generation-timeout",
        type=positive_int,
        default=600,
        help="per-generation-attempt timeout in seconds",
    )
    parser.add_argument(
        "--max-retries",
        type=non_negative_int,
        default=2,
        help="retries after the initial generation attempt",
    )
    parser.add_argument(
        "--evaluate-existing",
        type=Path,
        help="rebuild and rescore an existing run without generation",
    )
    parser.add_argument(
        "--log-level",
        choices=("DEBUG", "INFO", "WARNING", "ERROR"),
        default="DEBUG",
        help="console log level; the run log always captures DEBUG",
    )
    args = parser.parse_args(argv)

    if args.evaluate_existing is None:
        if args.provider is None:
            parser.error("--provider is required for generation")
        if not args.model:
            parser.error("--model is required for generation")

    return Config(
        provider=args.provider,
        model=args.model,
        candidates=args.candidates,
        run_id=args.run_id,
        run_root=args.run_root,
        source=args.source,
        oracle=args.oracle,
        test_timeout=args.timeout,
        generation_timeout=args.generation_timeout,
        max_retries=args.max_retries,
        evaluate_existing=args.evaluate_existing,
        codex_bin=os.environ.get("CODEX_BIN", "codex"),
        opencode_bin=os.environ.get("OPENCODE_BIN", "opencode"),
        antigravity_bin=os.environ.get("ANTIGRAVITY_BIN", "agy"),
        log_level=args.log_level,
    )
