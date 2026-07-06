from __future__ import annotations

import json
import subprocess
import tempfile
import time
from pathlib import Path

from .base import FILES_JSON_SCHEMA, GenerationError, GenerationRequest, ResponseFormat
from ..logging_utils import logger


class CliGenerator:
    provider: str
    prompt_via_stdin = False

    def __init__(self, executable: str, model: str, timeout: int) -> None:
        self.executable = executable
        self.model = model
        self.timeout = timeout

    def command(
        self, request: GenerationRequest, schema_path: Path | None
    ) -> list[str]:
        raise NotImplementedError

    def generate(self, request: GenerationRequest) -> str:
        with tempfile.TemporaryDirectory(prefix=f"spectra-{self.provider}-") as temp:
            schema_path: Path | None = None
            if request.response_format is ResponseFormat.FILES_JSON:
                schema_path = Path(temp) / "files.schema.json"
                schema_path.write_text(json.dumps(FILES_JSON_SCHEMA))
            try:
                started = time.monotonic()
                logger.debug(
                    "starting CLI provider=%s executable=%s model=%s request=%s cwd=%s",
                    self.provider,
                    self.executable,
                    self.model,
                    request.name,
                    temp,
                )
                result = subprocess.run(
                    self.command(request, schema_path),
                    input=request.prompt if self.prompt_via_stdin else None,
                    text=True,
                    capture_output=True,
                    timeout=self.timeout,
                    check=False,
                    cwd=temp,
                )
            except subprocess.TimeoutExpired as exc:
                raise GenerationError(
                    f"{self.provider} timed out after {self.timeout}s"
                ) from exc
            if result.returncode != 0:
                detail = result.stderr.strip()[-2000:]
                raise GenerationError(
                    f"{self.provider} exited with {result.returncode}: {detail}"
                )
            if not result.stdout.strip():
                raise GenerationError(f"{self.provider} returned an empty response")
            logger.debug(
                "CLI completed provider=%s request=%s duration=%.3fs stdout_chars=%d stderr_chars=%d",
                self.provider,
                request.name,
                time.monotonic() - started,
                len(result.stdout),
                len(result.stderr),
            )
            return result.stdout.strip()

    def version(self) -> str:
        try:
            result = subprocess.run(
                [self.executable, "--version"],
                text=True,
                capture_output=True,
                timeout=10,
                check=False,
            )
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise GenerationError(
                f"could not determine {self.provider} version: {exc}"
            ) from exc
        return (result.stdout or result.stderr).strip()


class CodexGenerator(CliGenerator):
    provider = "codex"
    prompt_via_stdin = True

    def command(
        self, request: GenerationRequest, schema_path: Path | None
    ) -> list[str]:
        command = [
            self.executable,
            "exec",
            "-",
            "--model",
            self.model,
            "--sandbox",
            "read-only",
            "--ephemeral",
            "--ignore-user-config",
            "--skip-git-repo-check",
            "--color",
            "never",
        ]
        if schema_path is not None:
            command.extend(["--output-schema", str(schema_path)])
        return command


class OpenCodeGenerator(CliGenerator):
    provider = "opencode"

    def command(
        self, request: GenerationRequest, schema_path: Path | None
    ) -> list[str]:
        del schema_path
        return [
            self.executable,
            "run",
            "--model",
            self.model,
            "--pure",
            request.prompt,
        ]


class AntigravityGenerator(CliGenerator):
    provider = "antigravity"

    def command(
        self, request: GenerationRequest, schema_path: Path | None
    ) -> list[str]:
        del schema_path
        return [
            self.executable,
            "--model",
            self.model,
            "--sandbox",
            "--print",
            request.prompt,
        ]
