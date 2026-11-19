import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location('reserve', ROOT / 'scripts/prepare-vt-reserve-overlay.py')
RESERVE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RESERVE)


class OverlayAdmission(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.stock = Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())
        cls.lock = json.loads((RESERVE.DOCUMENTS / 'overlay-lock.json').read_text())

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='vt-reserve-admission-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def copied_sdk(self):
        sdk = self.root / 'sdk'
        for name in ['VERSION', *self.lock['files']]:
            target = sdk / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(self.stock / name, target)
        return sdk

    def test_exact_real_sources_and_single_broken_shrink_control(self):
        out = self.root / 'overlay'
        result = RESERVE.prepare(self.stock, out)
        self.assertTrue(result['stockSourcesUnchanged'])
        normal = json.loads((out / 'reserve-overlay.json').read_text())['Replace']
        negative = json.loads((out / 'negative-overlay.json').read_text())['Replace']
        changed = [name for name in normal if normal[name] != negative[name]]
        self.assertEqual(changed, [str(self.stock / self.lock['negativeControl']['path'])])
        expected = Path(normal[changed[0]]).read_text().replace(self.lock['negativeControl']['removeExactly'], '', 1)
        self.assertEqual(Path(negative[changed[0]]).read_text(), expected)
        for name, identity in self.lock['files'].items():
            self.assertEqual(RESERVE.sha(self.stock / name), identity['beforeSHA256'])

    def test_changed_stock_source_refuses_before_creating_output(self):
        sdk = self.copied_sdk()
        changed = sdk / next(iter(self.lock['files']))
        changed.write_bytes(changed.read_bytes() + b'\n// changed\n')
        with self.assertRaisesRegex(ValueError, 'stock SDK source identity differs'):
            RESERVE.prepare(sdk, self.root / 'out')
        self.assertFalse((self.root / 'out').exists())

    def test_different_version_refuses(self):
        sdk = self.copied_sdk()
        (sdk / 'VERSION').write_text('go1.27.99\n')
        with self.assertRaisesRegex(ValueError, 'version differs'):
            RESERVE.prepare(sdk, self.root / 'out')

    def test_changed_patch_refuses_before_output(self):
        documents = self.root / 'documents'
        shutil.copytree(RESERVE.DOCUMENTS, documents)
        patch = documents / 'go-reserve.patch'
        patch.write_bytes(patch.read_bytes() + b'\n')
        with self.assertRaisesRegex(ValueError, 'patch identity differs'):
            RESERVE.prepare(self.stock, self.root / 'out', documents)
        self.assertFalse((self.root / 'out').exists())

    def test_sdk_output_and_existing_evidence_refused(self):
        sdk = self.copied_sdk()
        with self.assertRaisesRegex(ValueError, 'outside the stock SDK'):
            RESERVE.prepare(sdk, sdk / 'out')
        output = self.root / 'evidence'
        output.mkdir()
        sentinel = output / 'retain.txt'
        sentinel.write_text('prior evidence')
        with self.assertRaisesRegex(ValueError, 'already exists'):
            RESERVE.prepare(sdk, output)
        self.assertEqual(sentinel.read_text(), 'prior evidence')


if __name__ == '__main__':
    unittest.main()
