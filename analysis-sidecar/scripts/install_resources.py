"""Install and verify the exact NLTK resources used by the sidecar."""

from __future__ import annotations

import argparse
from pathlib import Path

import nltk

from meking_analysis_sidecar.resources import (
    NLTK_RESOURCE_IDENTITIES,
    directory_digest,
)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--destination", required=True, type=Path)
    args = parser.parse_args()
    destination = args.destination.resolve()
    destination.mkdir(parents=True, exist_ok=True)
    for resource, relative, expected in NLTK_RESOURCE_IDENTITIES.values():
        nltk.download(
            resource,
            download_dir=str(destination),
            quiet=True,
            raise_on_error=True,
        )
        actual = directory_digest(destination / relative)
        if actual != expected:
            raise RuntimeError(
                f"{resource} fingerprint {actual} does not match {expected}"
            )


if __name__ == "__main__":
    main()
