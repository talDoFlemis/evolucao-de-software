from pathlib import Path
from types import SimpleNamespace

from spectra_runner.generators import GenerationRequest, ResponseFormat
from spectra_runner.generators.cli import (
    AntigravityGenerator,
    CodexGenerator,
    OpenCodeGenerator,
)
from spectra_runner.generators.openrouter import OpenRouterGenerator


def test_codex_command_is_read_only_ephemeral_without_native_schema() -> None:
    generator = CodexGenerator("codex-custom", "gpt-test", 10)
    command = generator.command(
        GenerationRequest("candidate", "prompt", ResponseFormat.FILES_JSON),
        Path("schema.json"),
    )

    assert command[:2] == ["codex-custom", "exec"]
    assert ["--sandbox", "read-only"] == command[command.index("--sandbox") :][:2]
    assert "--ephemeral" in command
    assert "--output-schema" not in command


def test_opencode_command_uses_pure_mode_and_embedded_prompt() -> None:
    generator = OpenCodeGenerator("opencode-custom", "provider/model", 10)
    command = generator.command(GenerationRequest("spec", "the prompt"), None)

    assert command == [
        "opencode-custom",
        "run",
        "--model",
        "provider/model",
        "--pure",
        "the prompt",
    ]


def test_antigravity_command_uses_sandboxed_print_mode() -> None:
    generator = AntigravityGenerator("agy-custom", "Gemini Test", 10)
    command = generator.command(GenerationRequest("spec", "the prompt"), None)

    assert command == [
        "agy-custom",
        "--model",
        "Gemini Test",
        "--sandbox",
        "--print",
        "the prompt",
    ]


def test_openrouter_requests_strict_structured_output() -> None:
    captured: dict[str, object] = {}

    class Completions:
        def create(self, **kwargs: object) -> object:
            captured.update(kwargs)
            message = SimpleNamespace(content='{"files":[]}')
            return SimpleNamespace(choices=[SimpleNamespace(message=message)])

    generator = OpenRouterGenerator.__new__(OpenRouterGenerator)
    generator.model = "provider/model"
    generator.client = SimpleNamespace(chat=SimpleNamespace(completions=Completions()))

    generator.generate(
        GenerationRequest("candidate", "prompt", ResponseFormat.FILES_JSON)
    )

    response_format = captured["response_format"]
    assert isinstance(response_format, dict)
    assert response_format["type"] == "json_schema"
    assert response_format["json_schema"]["strict"] is True
