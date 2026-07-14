from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

from meshclaw import __version__


DEFAULT_GO_PACKAGE = "github.com/meshclaw/meshclaw/cmd/meshclaw"
WRAPPER_FLAGS = {
    "--install-binary",
    "--print-binary",
    "--no-auto-install",
}


def main() -> int:
    wrapper_flag = sys.argv[1] if len(sys.argv) > 1 and sys.argv[1] in WRAPPER_FLAGS else ""
    if wrapper_flag == "--install-binary":
        binary = install_meshclaw_binary()
        if binary is None:
            return 127
        print(binary)
        return 0

    binary = find_meshclaw_binary()
    if binary is None and wrapper_flag != "--no-auto-install" and auto_install_enabled():
        binary = install_meshclaw_binary()

    if wrapper_flag == "--print-binary":
        if binary is None:
            return 127
        print(binary)
        return 0

    if binary is None:
        print(binary_missing_message(), file=sys.stderr)
        return 127

    args = sys.argv[1:]
    if wrapper_flag == "--no-auto-install":
        args = args[1:]
    completed = subprocess.run([binary, *args])
    return completed.returncode


def find_meshclaw_binary() -> str | None:
    configured = os.environ.get("MESHCLAW_BIN")
    if configured and os.access(configured, os.X_OK):
        return configured

    current = Path(sys.argv[0]).resolve()
    for candidate in candidate_binary_paths():
        if candidate.exists() and os.access(candidate, os.X_OK):
            try:
                if candidate.resolve() == current:
                    continue
            except OSError:
                pass
            if not is_meshclaw_runtime(candidate):
                continue
            return str(candidate)

    path = os.environ.get("PATH", "")
    for directory in path.split(os.pathsep):
        if not directory:
            continue
        candidate = Path(directory) / "meshclaw"
        if not candidate.exists() or not os.access(candidate, os.X_OK):
            continue
        try:
            if candidate.resolve() == current:
                continue
        except OSError:
            continue
        if is_meshclaw_runtime(candidate):
            return str(candidate)

    fallback = shutil.which("meshclaw")
    if fallback:
        try:
            if Path(fallback).resolve() != current and is_meshclaw_runtime(Path(fallback)):
                return fallback
        except OSError:
            if is_meshclaw_runtime(Path(fallback)):
                return fallback

    return None


def is_meshclaw_runtime(path: Path) -> bool:
    try:
        completed = subprocess.run(
            [str(path), "--version"],
            check=False,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            timeout=2,
        )
    except (OSError, subprocess.TimeoutExpired):
        return False
    if completed.returncode != 0:
        return False
    return re.match(r"^\d+\.\d+\.\d+\s*$", completed.stdout) is not None


def candidate_binary_paths() -> list[Path]:
    paths: list[Path] = []
    for value in (
        os.environ.get("MESHCLAW_INSTALL_DIR"),
        str(Path.home() / ".local" / "bin"),
        str(Path.home() / "go" / "bin"),
        "/Users/example/bin",
        "/usr/local/bin",
        "/opt/homebrew/bin",
    ):
        if not value:
            continue
        paths.append(Path(value) / "meshclaw")
    return paths


def auto_install_enabled() -> bool:
    value = os.environ.get("MESHCLAW_AUTO_INSTALL", "1").strip().lower()
    return value not in {"0", "false", "no", "off"}


def install_meshclaw_binary() -> str | None:
    go = shutil.which("go")
    if not go:
        print(binary_missing_message(), file=sys.stderr)
        return None

    install_dir = Path(
        os.environ.get("MESHCLAW_INSTALL_DIR", str(Path.home() / ".local" / "bin"))
    ).expanduser()
    try:
        install_dir.mkdir(parents=True, exist_ok=True)
    except OSError as exc:
        print(f"meshclaw: cannot create install dir {install_dir}: {exc}", file=sys.stderr)
        return None

    package = os.environ.get("MESHCLAW_GO_PACKAGE", DEFAULT_GO_PACKAGE)
    version = os.environ.get("MESHCLAW_GO_VERSION", "latest")
    target = f"{package}@{version}"
    env = os.environ.copy()
    env["GOBIN"] = str(install_dir)

    print(f"meshclaw: installing Go binary with `{go} install {target}`", file=sys.stderr)
    print(f"meshclaw: GOBIN={install_dir}", file=sys.stderr)
    try:
        subprocess.run([go, "install", target], check=True, env=env)
    except subprocess.CalledProcessError as exc:
        print(f"meshclaw: go install failed with exit code {exc.returncode}", file=sys.stderr)
        print(binary_missing_message(auto_install_failed=True), file=sys.stderr)
        return None

    binary = install_dir / "meshclaw"
    if binary.exists() and os.access(binary, os.X_OK):
        return str(binary)
    print(f"meshclaw: go install finished but {binary} is not executable", file=sys.stderr)
    return None


def binary_missing_message(auto_install_failed: bool = False) -> str:
    prefix = "meshclaw Go binary is not installed."
    if auto_install_failed:
        prefix = "meshclaw Go binary could not be installed automatically."
    return (
        f"{prefix}\n"
        "\n"
        "Options:\n"
        "  meshclaw --install-binary\n"
        "  meshclaw init\n"
        "  go install github.com/meshclaw/meshclaw/cmd/meshclaw@latest\n"
        "  MESHCLAW_BIN=/path/to/meshclaw meshclaw help\n"
        "\n"
        "Environment:\n"
        "  MESHCLAW_AUTO_INSTALL=0       disable automatic Go install\n"
        "  MESHCLAW_INSTALL_DIR=DIR      install target for automatic bootstrap\n"
        "  MESHCLAW_GO_VERSION=latest    Go module version/ref to install\n"
        f"  wrapper_version={__version__}"
    )


if __name__ == "__main__":
    raise SystemExit(main())
