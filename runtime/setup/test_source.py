import hashlib
import json
from pathlib import Path
from types import SimpleNamespace
import tempfile
import unittest

import source


class DownloadTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.snapshot = self.root/'cache'
        self.snapshot.mkdir()
        (self.snapshot/'weights.safetensors').write_bytes(b'weights')
        (self.snapshot/'LICENSE').write_text('Apache-2.0')
        self.revision = 'a'*40
        self.calls = []
        self.info = SimpleNamespace(sha=self.revision, siblings=[
            SimpleNamespace(rfilename='weights.safetensors', lfs=SimpleNamespace(sha256=hashlib.sha256(b'weights').hexdigest())),
            SimpleNamespace(rfilename='LICENSE', lfs=None)])
        self.api = SimpleNamespace(model_info=lambda *args, **kwargs: self.info)

    def snapshot_download(self, *args, **kwargs):
        self.calls.append(kwargs)
        return str(self.snapshot)

    def test_revision_is_saved_before_download_and_reused_offline(self):
        def fail(*args, **kwargs):
            self.assertEqual((self.root/'source-revision.txt').read_text().strip(), self.revision)
            raise OSError('interrupted')
        with self.assertRaises(OSError):
            source.download(self.root, api=self.api, snapshot=fail)
        result = source.download(self.root, api=self.api, snapshot=self.snapshot_download)
        self.assertEqual(self.calls[0]['revision'], self.revision)
        self.assertEqual(result['revision'], self.revision)
        cached = source.download(self.root, api=None, snapshot=None)
        self.assertEqual(cached, result)

    def test_weight_checksum_mismatch_is_rejected(self):
        (self.snapshot/'weights.safetensors').write_bytes(b'corrupt')
        with self.assertRaisesRegex(ValueError, 'SOURCE_INTEGRITY'):
            source.download(self.root, api=self.api, snapshot=self.snapshot_download)
        self.assertFalse((self.root/'source.json').exists())

    def test_cached_source_is_rehashed(self):
        source.download(self.root, api=self.api, snapshot=self.snapshot_download)
        (self.snapshot/'LICENSE').write_text('changed')
        with self.assertRaisesRegex(ValueError, 'SOURCE_INTEGRITY'):
            source.download(self.root)

    def test_mutable_revision_is_rejected(self):
        (self.root/'source-revision.txt').write_text('main')
        with self.assertRaisesRegex(ValueError, 'SOURCE_REVISION'):
            source.download(self.root, api=self.api, snapshot=self.snapshot_download)

    def test_original_manual_snapshot_revision_is_adopted(self):
        (self.root/'source-revision.txt').write_text(self.revision+'\n')
        source.download(self.root, api=self.api, snapshot=self.snapshot_download)
        self.assertEqual(self.calls[0]['revision'], self.revision)


if __name__ == '__main__':
    unittest.main()
