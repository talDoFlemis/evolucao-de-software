from __future__ import annotations

import logging
import sys
import threading
import time
from contextlib import contextmanager
from collections.abc import Iterator
from pathlib import Path


LOGGER_NAME = "spectra_runner"
logger = logging.getLogger(LOGGER_NAME)


class UtcFormatter(logging.Formatter):
    converter = time.gmtime


def configure_logging(log_file: Path, console_level: str, append: bool) -> None:
    logger.setLevel(logging.DEBUG)
    logger.propagate = False
    logger.handlers.clear()

    formatter = UtcFormatter(
        "%(asctime)sZ %(levelname)-7s %(name)s: %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%S",
    )
    console = logging.StreamHandler(sys.stderr)
    console.setLevel(console_level)
    console.setFormatter(formatter)
    logger.addHandler(console)

    file_handler = logging.FileHandler(log_file, mode="a" if append else "w")
    file_handler.setLevel(logging.DEBUG)
    file_handler.setFormatter(formatter)
    logger.addHandler(file_handler)


@contextmanager
def heartbeat(
    operation: str, timeout_seconds: int, interval_seconds: int = 30
) -> Iterator[None]:
    stopped = threading.Event()
    started = time.monotonic()

    def report() -> None:
        while not stopped.wait(interval_seconds):
            elapsed = time.monotonic() - started
            logger.info(
                "%s still running; elapsed=%.0fs timeout=%ds",
                operation,
                elapsed,
                timeout_seconds,
            )

    thread = threading.Thread(target=report, name="spectra-heartbeat", daemon=True)
    thread.start()
    try:
        yield
    finally:
        stopped.set()
        thread.join(timeout=1)
