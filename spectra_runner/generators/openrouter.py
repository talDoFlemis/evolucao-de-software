from __future__ import annotations

import os
from importlib.metadata import version

from openai import OpenAI

from .base import FILES_JSON_SCHEMA, GenerationError, GenerationRequest, ResponseFormat


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
        return content.strip()

    def version(self) -> str:
        return f"openai-python {version('openai')}"
