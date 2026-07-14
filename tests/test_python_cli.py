import os
import stat
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from meshclaw import __version__
from meshclaw import cli


class MeshClawPythonWrapperTest(unittest.TestCase):
    def test_auto_install_enabled_defaults_on(self):
        with mock.patch.dict(os.environ, {}, clear=True):
            self.assertTrue(cli.auto_install_enabled())

    def test_auto_install_can_be_disabled(self):
        for value in ("0", "false", "no", "off"):
            with self.subTest(value=value):
                with mock.patch.dict(os.environ, {"MESHCLAW_AUTO_INSTALL": value}, clear=True):
                    self.assertFalse(cli.auto_install_enabled())

    def test_candidate_binary_paths_respect_install_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.dict(os.environ, {"MESHCLAW_INSTALL_DIR": tmp}, clear=True):
                paths = cli.candidate_binary_paths()
        self.assertEqual(paths[0], Path(tmp) / "meshclaw")

    def test_find_meshclaw_binary_uses_configured_binary(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = Path(tmp) / "meshclaw"
            binary.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            binary.chmod(binary.stat().st_mode | stat.S_IXUSR)
            env = {"MESHCLAW_BIN": str(binary), "PATH": ""}
            with mock.patch.dict(os.environ, env, clear=True):
                self.assertEqual(cli.find_meshclaw_binary(), str(binary))

    def test_find_meshclaw_binary_skips_wrapper_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            wrapper = Path(tmp) / "meshclaw"
            wrapper.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            wrapper.chmod(wrapper.stat().st_mode | stat.S_IXUSR)
            with mock.patch.dict(os.environ, {"PATH": tmp}, clear=True):
                with mock.patch.object(sys, "argv", [str(wrapper)]):
                    with mock.patch.object(cli, "candidate_binary_paths", return_value=[]):
                        with mock.patch.object(cli.shutil, "which", return_value=None):
                            self.assertIsNone(cli.find_meshclaw_binary())

    def test_find_meshclaw_binary_prefers_candidate_before_path_wrapper(self):
        with tempfile.TemporaryDirectory() as tmp:
            path_dir = Path(tmp) / "path"
            bin_dir = Path(tmp) / "bin"
            path_dir.mkdir()
            bin_dir.mkdir()
            path_wrapper = path_dir / "meshclaw"
            path_wrapper.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            path_wrapper.chmod(path_wrapper.stat().st_mode | stat.S_IXUSR)
            go_binary = bin_dir / "meshclaw"
            go_binary.write_text("#!/bin/sh\nprintf '1.2.42\\n'\n", encoding="utf-8")
            go_binary.chmod(go_binary.stat().st_mode | stat.S_IXUSR)
            with mock.patch.dict(os.environ, {"PATH": str(path_dir)}, clear=True):
                with mock.patch.object(sys, "argv", [str(path_wrapper)]):
                    with mock.patch.object(cli, "candidate_binary_paths", return_value=[go_binary]):
                        self.assertEqual(cli.find_meshclaw_binary(), str(go_binary))

    def test_find_meshclaw_binary_skips_stale_candidate(self):
        with tempfile.TemporaryDirectory() as tmp:
            stale = Path(tmp) / "meshclaw"
            stale.write_text("#!/bin/sh\necho 'old cobra cli' >&2\nexit 1\n", encoding="utf-8")
            stale.chmod(stale.stat().st_mode | stat.S_IXUSR)
            with mock.patch.dict(os.environ, {"PATH": ""}, clear=True):
                with mock.patch.object(cli, "candidate_binary_paths", return_value=[stale]):
                    with mock.patch.object(cli.shutil, "which", return_value=None):
                        self.assertIsNone(cli.find_meshclaw_binary())

    def test_missing_message_is_actionable_and_versioned(self):
        message = cli.binary_missing_message()
        self.assertIn("meshclaw --install-binary", message)
        self.assertIn("meshclaw init", message)
        self.assertIn("MESHCLAW_BIN=/path/to/meshclaw", message)
        self.assertIn(f"wrapper_version={__version__}", message)


if __name__ == "__main__":
    unittest.main()
