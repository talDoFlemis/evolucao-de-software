from __future__ import annotations

import json
from dataclasses import replace
from pathlib import Path

import pytest

from spectra_runner.config import Config
from spectra_runner.generation import (
    GenerationService,
    parse_generated_files,
    write_generated_files,
)
from spectra_runner.generators import GenerationError, GenerationRequest, ResponseFormat


VALID_RESPONSE = json.dumps(
    {
        "files": [
            {"path": "go.mod", "content": "module candidate\n\ngo 1.23\n"},
            {"path": "cmd/main.go", "content": "package main\nfunc main() {}\n"},
        ]
    }
)


def config(tmp_path: Path) -> Config:
    source = tmp_path / "cat.c"
    source.write_text("int main(void) {}")
    return Config(
        provider="codex",
        model="test-model",
        candidates=1,
        run_id="test",
        run_root=tmp_path,
        source=source,
        oracle=Path("/usr/bin/cat"),
        test_timeout=1,
        generation_timeout=1,
        max_retries=2,
        evaluate_existing=None,
        codex_bin="codex",
        opencode_bin="opencode",
        antigravity_bin="agy",
        log_level="INFO",
    )


@pytest.mark.parametrize(
    "path",
    ["../main.go", "/main.go", "context/main.go", "README.md", "go.sum", "a\\b.go"],
)
def test_rejects_disallowed_paths(path: str) -> None:
    response = json.dumps(
        {
            "files": [
                {"path": "go.mod", "content": "module x"},
                {"path": path, "content": "x"},
            ]
        }
    )

    with pytest.raises(GenerationError):
        parse_generated_files(response)


def test_rejects_markdown_wrapped_json() -> None:
    with pytest.raises(GenerationError):
        parse_generated_files(f"```json\n{VALID_RESPONSE}\n```")


def test_writes_only_validated_files(tmp_path: Path) -> None:
    files = parse_generated_files(VALID_RESPONSE)
    write_generated_files(tmp_path, files)

    assert (tmp_path / "go.mod").is_file()
    assert (tmp_path / "cmd" / "main.go").is_file()


class StubGenerator:
    provider = "stub"
    model = "stub-model"

    def __init__(self, responses: list[str | Exception]) -> None:
        self.responses = responses
        self.calls = 0

    def generate(self, request: GenerationRequest) -> str:
        response = self.responses[self.calls]
        self.calls += 1
        if isinstance(response, Exception):
            raise response
        return response

    def version(self) -> str:
        return "stub 1"


def test_retries_invalid_json_and_records_manifest(tmp_path: Path) -> None:
    run = tmp_path / "run"
    logs = run / "logs"
    logs.mkdir(parents=True)
    generator = StubGenerator(["not-json", VALID_RESPONSE])
    service = GenerationService(
        generator, config(tmp_path), run, logs, sleep=lambda _: None
    )

    response = service.generate(
        GenerationRequest("candidate", "prompt", ResponseFormat.FILES_JSON)
    )

    assert response == VALID_RESPONSE
    assert generator.calls == 2
    manifest = json.loads((run / "run.json").read_text())
    assert [call["status"] for call in manifest["calls"]] == ["failed", "success"]


def test_exhausts_configured_retries(tmp_path: Path) -> None:
    run = tmp_path / "run"
    logs = run / "logs"
    logs.mkdir(parents=True)
    cfg = replace(config(tmp_path), max_retries=1)
    generator = StubGenerator([GenerationError("one"), GenerationError("two")])
    service = GenerationService(generator, cfg, run, logs, sleep=lambda _: None)

    with pytest.raises(GenerationError, match="failed after 2 attempts"):
        service.generate(GenerationRequest("spec", "prompt"))
