from __future__ import annotations

import os
import time
from importlib.metadata import version

from openai import OpenAI

from .base import FILES_JSON_SCHEMA, GenerationError, GenerationRequest, ResponseFormat
from ..logging_utils import logger


class OpenRouterGenerator:
    provider = "openrouter"

    def __init__(self, model: str, timeout: int) -> None:
        api_key = os.environ.get("OPENROUTER_API_KEY")
        if not api_key:
            raise GenerationError("OPENROUTER_API_KEY is required for OpenRouter")
        self.model = model
        self.client = OpenAI(
            api_key=api_key,
            base_url="https://openrouter.ai/api/v1",
            timeout=timeout,
            max_retries=0,
        )

    def generate(self, request: GenerationRequest) -> str:
        kwargs: dict[str, object] = {}
        if request.response_format is ResponseFormat.FILES_JSON:
            kwargs["response_format"] = {
                "type": "json_schema",
                "json_schema": {
                    "name": "generated_files",
                    "strict": True,
                    "schema": FILES_JSON_SCHEMA,
                },
            }
        try:
            started = time.monotonic()
            logger.debug(
                "starting OpenRouter request=%s model=%s structured=%s",
                request.name,
                self.model,
                request.response_format is ResponseFormat.FILES_JSON,
            )
            response = self.client.chat.completions.create(
                model=self.model,
                messages=[{"role": "user", "content": request.prompt}],
                **kwargs,  # type: ignore[arg-type]
            )
        except Exception as exc:
            raise GenerationError(f"OpenRouter request failed: {exc}") from exc
        content = response.choices[0].message.content
        if not content:
            raise GenerationError("OpenRouter returned an empty response")
        usage = getattr(response, "usage", None)
        logger.debug(
            "OpenRouter completed request=%s duration=%.3fs prompt_tokens=%s completion_tokens=%s",
            request.name,
            time.monotonic() - started,
            getattr(usage, "prompt_tokens", None),
            getattr(usage, "completion_tokens", None),
        )
        return content.strip()

    def version(self) -> str:
        return f"openai-python {version('openai')}"
