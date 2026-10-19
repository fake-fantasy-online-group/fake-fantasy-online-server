"""Fast, dependency-free tests for the artifact boundary (run on every PR)."""
import hashlib
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import build_server as build


class BundleTests(unittest.TestCase):
    def stage(self, root, goos):
        stage = root / 'stage'
        stage.mkdir()
        for name in build.bundle_names(goos) - {'SHA256SUMS'}:
            (stage / name).write_bytes((name + '\n').encode())
        (stage / 'SHA256SUMS').write_text(build.checksum_text(list(stage.iterdir())))
        return stage

    def test_both_platform_allowlists_and_hashes(self):
        for goos in build.TARGETS:
            with self.subTest(goos=goos), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                stage = self.stage(root, goos)
                archive = root / 'bundle.zip'
                build.create_bundle(stage, archive, goos)
                build.verify_bundle(archive, goos)
                with zipfile.ZipFile(archive) as z:
                    self.assertEqual(set(z.namelist()), build.bundle_names(goos))
                    self.assertNotIn('controller', z.namelist())
                    self.assertTrue(all('/' not in name for name in z.namelist()))
                    info = z.getinfo('gameserver' + ('.exe' if goos == 'windows' else ''))
                    self.assertEqual((info.external_attr >> 16) & 0o777, 0o755)
                # ZIP metadata and order must be stable for identical input.
                second = root / 'bundle-2.zip'
                build.create_bundle(stage, second, goos)
                self.assertEqual(archive.read_bytes(), second.read_bytes())

    def test_rejects_unexpected_files_and_directories(self):
        for extra in ('server_private.pem', '.env', 'backup.dump', 'source.go', 'assets'):
            with self.subTest(extra=extra), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                stage = self.stage(root, 'linux')
                (stage / extra).write_text('must never be archived')
                with self.assertRaisesRegex(ValueError, 'unexpected bundle contents'):
                    build.create_bundle(stage, root / 'bundle.zip', 'linux')

    def test_rejects_symlink_in_allowlist(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            stage = self.stage(root, 'linux')
            (root / 'outside').write_text('secret')
            (stage / 'README.txt').unlink()
            (stage / 'README.txt').symlink_to(root / 'outside')
            with self.assertRaisesRegex(ValueError, 'unexpected bundle contents'):
                build.create_bundle(stage, root / 'bundle.zip', 'linux')

    def test_detects_tampered_binary(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            stage = self.stage(root, 'linux')
            (stage / 'gameserver').write_text('changed after hashing')
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                build.create_bundle(stage, root / 'bundle.zip', 'linux')

    def test_checksums_are_sorted_and_use_only_basenames(self):
        with tempfile.TemporaryDirectory() as temp:
            paths = [Path(temp) / name for name in ('b.zip', 'a.zip')]
            for path in paths:
                path.write_bytes(b'bytes')
            digest = hashlib.sha256(b'bytes').hexdigest()
            self.assertEqual(build.checksum_text(paths),
                             f'{digest}  a.zip\n{digest}  b.zip\n')

    def test_rejects_dirty_checkout_or_abbreviated_commit(self):
        with patch.object(build, 'run', return_value=' M server/go.mod'):
            with self.assertRaisesRegex(ValueError, 'dirty checkout'):
                build.require_clean_checkout()
        with patch.object(build, 'run', side_effect=['', 'abcdef0']):
            with self.assertRaisesRegex(ValueError, 'full 40-character'):
                build.require_clean_checkout()
        with patch.object(build, 'run', side_effect=['', 'a' * 40]):
            self.assertEqual(build.require_clean_checkout(), 'a' * 40)

    def test_init_warning_and_no_runtime_execution(self):
        self.assertIn('EMPTY DEVELOPMENT DATABASE INITIALIZER', build.README)
        self.assertIn('NOT a production\nincremental migration tool', build.README)
        self.assertIn('required init SQL is deliberately not bundled', build.README)


if __name__ == '__main__':
    unittest.main()
