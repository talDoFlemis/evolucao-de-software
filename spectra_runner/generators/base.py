from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Protocol


class ResponseFormat(Enum):
    TEXT = "text"
    FILES_JSON = "files_json"


@dataclass(frozen=True)
class GenerationRequest:
    name: str
    prompt: str
    response_format: ResponseFormat = ResponseFormat.TEXT


class GenerationError(RuntimeError):
    pass


class Generator(Protocol):
    provider: str
    model: str

    def generate(self, request: GenerationRequest) -> str: ...

    def version(self) -> str: ...


FILES_JSON_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "required": ["files"],
    "properties": {
        "files": {
            "type": "array",
            "minItems": 2,
            "items": {
                "type": "object",
                "additionalProperties": False,
                "required": ["path", "content"],
                "properties": {
                    "path": {"type": "string", "minLength": 1},
                    "content": {"type": "string"},
                },
            },
        }
    },
}
