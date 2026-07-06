from __future__ import annotations

from spectra_runner.config import Config

from .base import Generator
from .cli import AntigravityGenerator, CodexGenerator, OpenCodeGenerator
from .openrouter import OpenRouterGenerator


def create_generator(config: Config) -> Generator:
    assert config.provider is not None and config.model is not None
    if config.provider == "openrouter":
        return OpenRouterGenerator(config.model, config.generation_timeout)
    if config.provider == "codex":
        return CodexGenerator(config.codex_bin, config.model, config.generation_timeout)
    if config.provider == "opencode":
        return OpenCodeGenerator(
            config.opencode_bin, config.model, config.generation_timeout
        )
    if config.provider == "antigravity":
        return AntigravityGenerator(
            config.antigravity_bin, config.model, config.generation_timeout
        )
    raise ValueError(f"unsupported provider: {config.provider}")
