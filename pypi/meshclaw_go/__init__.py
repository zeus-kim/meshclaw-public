"""meshclaw — download the official Go binary from GitHub Releases."""

from __future__ import annotations

import os
import platform
import stat
import subprocess
import sys
import urllib.request
from pathlib import Path

__version__ = "1.2.54"

REPO = "zeus-kim/meshclaw-public"


def _platform_bits() -> tuple[str, str]:
    system = platform.system().lower()
    machine = platform.machine().lower()
    os_name = "darwin" if system == "darwin" else "linux"
    if machine in ("arm64", "aarch64"):
        arch = "arm64"
    else:
        arch = "amd64"
    return os_name, arch


def _binary_dir() -> Path:
    local_bin = Path.home() / ".local" / "bin"
    local_bin.mkdir(parents=True, exist_ok=True)
    return local_bin


def _download(name: str) -> Path:
    os_name, arch = _platform_bits()
    url = f"https://github.com/{REPO}/releases/latest/download/{name}-{os_name}-{arch}"
    dest = _binary_dir() / name
    if dest.exists() and os.access(dest, os.X_OK):
        return dest
    print(f"Downloading {name} from {url} ...", file=sys.stderr)
    tmp = dest.with_suffix(".tmp")
    urllib.request.urlretrieve(url, tmp)
    tmp.replace(dest)
    dest.chmod(dest.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    print(f"Installed {name} -> {dest}", file=sys.stderr)
    return dest


def main() -> None:
    try:
        binary = _download("meshclaw")
        proc = subprocess.run([str(binary)] + sys.argv[1:])
        raise SystemExit(proc.returncode)
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 — user-facing installer
        print(f"Error: {exc}", file=sys.stderr)
        print(f"Manual install: https://github.com/{REPO}/releases", file=sys.stderr)
        raise SystemExit(1) from exc
