from __future__ import annotations

import hashlib
import json
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath
from typing import Callable

from .config import Config
from .generators import GenerationError, GenerationRequest, Generator, ResponseFormat
from .logging_utils import heartbeat, logger


@dataclass(frozen=True)
class GeneratedFile:
    path: PurePosixPath
    content: str


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()


def parse_generated_files(response: str) -> list[GeneratedFile]:
    try:
        document = json.loads(response)
    except json.JSONDecodeError as exc:
        raise GenerationError(f"candidate response is not strict JSON: {exc}") from exc
    if not isinstance(document, dict) or set(document) != {"files"}:
        raise GenerationError("candidate JSON must contain only the 'files' property")
    raw_files = document["files"]
    if not isinstance(raw_files, list):
        raise GenerationError("candidate 'files' must be an array")

    files: list[GeneratedFile] = []
    seen: set[PurePosixPath] = set()
    for item in raw_files:
        if not isinstance(item, dict) or set(item) != {"path", "content"}:
            raise GenerationError("each generated file needs only path and content")
        path_value, content = item["path"], item["content"]
        if not isinstance(path_value, str) or not isinstance(content, str):
            raise GenerationError("generated file path and content must be strings")
        if "\\" in path_value:
            raise GenerationError(
                f"generated path must use POSIX separators: {path_value}"
            )
        path = PurePosixPath(path_value)
        if (
            path.is_absolute()
            or not path.parts
            or any(p in ("", ".", "..") for p in path.parts)
        ):
            raise GenerationError(f"unsafe generated path: {path_value}")
        if path in seen:
            raise GenerationError(f"duplicate generated path: {path_value}")
        if path.parts[0] == "context":
            raise GenerationError(f"generated path is not allowed: {path_value}")
        if path == PurePosixPath("go.mod"):
            pass
        elif path.suffix != ".go":
            raise GenerationError(f"generated path is not allowed: {path_value}")
        seen.add(path)
        files.append(GeneratedFile(path, content))

    if PurePosixPath("go.mod") not in seen:
        raise GenerationError("candidate must contain one root-level go.mod")
    if not any(file.path.suffix == ".go" for file in files):
        raise GenerationError("candidate must contain at least one .go file")
    return files


def write_generated_files(package_dir: Path, files: list[GeneratedFile]) -> None:
    root = package_dir.resolve()
    for generated in files:
        target = package_dir.joinpath(*generated.path.parts)
        resolved = target.resolve()
        if not resolved.is_relative_to(root):
            raise GenerationError(f"generated path escapes package: {generated.path}")
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(generated.content)


class GenerationService:
    def __init__(
        self,
        generator: Generator,
        config: Config,
        run_dir: Path,
        logs_dir: Path,
        sleep: Callable[[float], None] = time.sleep,
    ) -> None:
        self.generator = generator
        self.config = config
        self.run_dir = run_dir
        self.logs_dir = logs_dir
        self.sleep = sleep
        self.calls: list[dict[str, object]] = []
        self.started_at = datetime.now(timezone.utc).isoformat()
        self.generator_version = generator.version()
        logger.debug(
            "initialized generator provider=%s model=%s version=%s",
            generator.provider,
            generator.model,
            self.generator_version,
        )

    def generate(self, request: GenerationRequest) -> str:
        prompt_hash = sha256_text(request.prompt)
        last_error: Exception | None = None
        for attempt in range(1, self.config.max_retries + 2):
            started = datetime.now(timezone.utc)
            logger.info(
                "generation step=%s attempt=%d/%d format=%s prompt_chars=%d",
                request.name,
                attempt,
                self.config.max_retries + 1,
                request.response_format.value,
                len(request.prompt),
            )
            try:
                with heartbeat(
                    f"generation step={request.name}", self.config.generation_timeout
                ):
                    response = self.generator.generate(request)
                if request.response_format is ResponseFormat.FILES_JSON:
                    parse_generated_files(response)
                self._record(request, prompt_hash, attempt, started, "success", None)
                self._write_log(request.name, attempt, response)
                self.write_manifest()
                logger.info(
                    "generation completed step=%s attempt=%d duration=%.3fs response_chars=%d",
                    request.name,
                    attempt,
                    (datetime.now(timezone.utc) - started).total_seconds(),
                    len(response),
                )
                return response
            except (GenerationError, ValueError) as exc:
                last_error = exc
                self._record(request, prompt_hash, attempt, started, "failed", str(exc))
                self._write_log(request.name, attempt, str(exc), error=True)
                self.write_manifest()
                logger.warning(
                    "generation failed step=%s attempt=%d duration=%.3fs error=%s",
                    request.name,
                    attempt,
                    (datetime.now(timezone.utc) - started).total_seconds(),
                    exc,
                )
                if attempt <= self.config.max_retries:
                    delay = min(2 ** (attempt - 1), 8)
                    logger.info(
                        "retrying generation step=%s after %ds", request.name, delay
                    )
                    self.sleep(delay)
        raise GenerationError(
            f"{request.name} failed after {self.config.max_retries + 1} attempts: {last_error}"
        )

    def write_manifest(self) -> None:
        source_hash = None
        if self.config.source.is_file():
            source_hash = hashlib.sha256(self.config.source.read_bytes()).hexdigest()
        manifest = {
            "provider": self.config.provider,
            "model": self.config.model,
            "started_at": self.started_at,
            "updated_at": datetime.now(timezone.utc).isoformat(),
            "generation_timeout_seconds": self.config.generation_timeout,
            "max_retries": self.config.max_retries,
            "source_sha256": source_hash,
            "generator_version": self.generator_version,
            "calls": self.calls,
        }
        (self.run_dir / "run.json").write_text(json.dumps(manifest, indent=2) + "\n")
        logger.debug("updated run manifest calls=%d", len(self.calls))

    def _record(
        self,
        request: GenerationRequest,
        prompt_hash: str,
        attempt: int,
        started: datetime,
        status: str,
        error: str | None,
    ) -> None:
        self.calls.append(
            {
                "name": request.name,
                "format": request.response_format.value,
                "prompt_sha256": prompt_hash,
                "attempt": attempt,
                "status": status,
                "started_at": started.isoformat(),
                "duration_seconds": round(
                    (datetime.now(timezone.utc) - started).total_seconds(), 3
                ),
                "error": error,
            }
        )

    def _write_log(
        self, name: str, attempt: int, value: str, error: bool = False
    ) -> None:
        suffix = "error" if error else "response"
        (self.logs_dir / f"{name}.attempt-{attempt}.{suffix}.log").write_text(value)
