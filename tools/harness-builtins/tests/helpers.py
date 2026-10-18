"""Shared by the extractor tests: saved sources instead of the network."""

import json
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent.parent
TESTDATA = HERE / "testdata"
sys.path.insert(0, str(HERE))

import importlib.util  # noqa: E402

from hb import fetch  # noqa: E402


def load_extractor(name):
    spec = importlib.util.spec_from_file_location(f"extractor_{name}", HERE / "extractors" / f"{name}.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def read(*parts) -> str:
    return TESTDATA.joinpath(*parts).read_text(encoding="utf-8")


def context(repo="", ref="", files=None, tree=None):
    """A Context whose pages are `files` (url or repo path -> text); `tree` lists repo paths."""
    files = dict(files or {})

    def get(url):
        if tree is not None and url == f"https://api.github.com/repos/{repo}/git/trees/{ref}?recursive=1":
            return json.dumps({"truncated": False, "tree": [{"path": p, "type": "blob"} for p in tree]})
        for key, text in files.items():
            if url == key or url == f"https://raw.githubusercontent.com/{repo}/{ref}/{key}":
                return text
        raise fetch.FetchError(f"{url}: not in the saved sources")

    return fetch.Context(repo, ref, get)
