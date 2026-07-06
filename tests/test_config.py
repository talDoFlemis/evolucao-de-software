from pathlib import Path

import pytest

from spectra_runner.config import parse_args


def test_generation_requires_provider_and_model() -> None:
    with pytest.raises(SystemExit):
        parse_args([])


def test_evaluation_does_not_require_generator_configuration() -> None:
    config = parse_args(["--evaluate-existing", "a-run"])

    assert config.evaluate_existing == Path("a-run")
    assert config.provider is None
    assert config.model is None


def test_generation_configuration() -> None:
    config = parse_args(
        [
            "--provider",
            "codex",
            "--model",
            "gpt-5",
            "--generation-timeout",
            "30",
            "--max-retries",
            "4",
        ]
    )

    assert config.provider == "codex"
    assert config.model == "gpt-5"
    assert config.generation_timeout == 30
    assert config.max_retries == 4
