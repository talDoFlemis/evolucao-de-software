from __future__ import annotations

from .config import parse_args
from .generation import GenerationService
from .generators import GenerationError, create_generator
from .workflow import die, resolve_paths, run


def main() -> None:
    config = parse_args()
    service = None
    try:
        if config.evaluate_existing is None:
            paths = resolve_paths(config)
            generator = create_generator(config)
            service = GenerationService(generator, config, paths.run, paths.logs)
        raise SystemExit(run(config, service))
    except (GenerationError, RuntimeError) as exc:
        die(str(exc))
