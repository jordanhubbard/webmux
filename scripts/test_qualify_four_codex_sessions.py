"""Receipt refusal controls only; these do not qualify native continuity."""
import importlib.util
import json
import os
from pathlib import Path
import runpy
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
import unittest
from unittest.mock import Mock
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('four_codex', Path(__file__).with_name('qualify-four-codex-sessions.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def cohort(counter=1):
    return [dict(session_id=f'session-{i}', thread_id=f'thread-{i}', turn_id=f'turn-{i}',
        viewer_epoch=f'viewer-{i}', provider_epoch=f'provider-{i}', daemon_epoch='daemon',
        turn_status='inProgress', management_thread_id=f'thread-{i}', selection_thread_id=f'thread-{i}',
        work_item_id=f'item-{i}', work_witness='authenticated-native-command-output-stream', work_counter=counter, work_epoch=f'worker-{i}', work_script_sha256=f'{i:064x}') for i in range(4)]


class ContinuityControls(unittest.TestCase):
    def test_private_ssh_shim_preserves_strict_trust_options_and_destination(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            trust = root / 'known hosts'
            trust.write_text('owned.example ssh-ed25519 public-fixture-key\n')
            trust.chmod(0o600)
            fixture = object.__new__(module.Fixture)
            fixture.root = root
            fixture.home, fixture.tmuxdir = root / 'native', root / 'tmux'
            fixture.env = {'PATH': '/usr/bin:/bin'}
            fixture.args = SimpleNamespace(webmux=root / 'webmux', ssh_host='owned.example',
                ssh_options=module.private_ssh_options(trust))
            fixture.configure_private_ssh(root / 'bin')
            shim = root / 'ssh-launch/ssh'
            with patch.object(sys, 'argv', [str(shim), 'owned.example', 'echo fixture']), \
                    patch.object(os, 'execv') as launch:
                runpy.run_path(str(shim), run_name='__main__')
            executable, argv = launch.call_args.args
            self.assertEqual(executable, '/usr/bin/ssh')
            self.assertIn('UserKnownHostsFile='+str(trust.resolve()), argv)
            self.assertIn('StrictHostKeyChecking=yes', argv)
            self.assertIn('UpdateHostKeys=no', argv)
            self.assertEqual(argv[-2], 'owned.example')
            self.assertTrue(argv[-1].endswith('; echo fixture'))

    def test_private_ssh_trust_requires_an_owned_nonwritable_regular_file(self):
        self.assertEqual(module.private_ssh_options(None), [])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            trust = root / 'known hosts'
            trust.write_text('owned.example ssh-ed25519 public-fixture-key\n')
            trust.chmod(0o600)
            options = module.private_ssh_options(trust)
            self.assertIn('UserKnownHostsFile='+str(trust.resolve()), options)
            self.assertIn('StrictHostKeyChecking=yes', options)
            self.assertIn('UpdateHostKeys=no', options)
            for mode in (0o620, 0o602):
                trust.chmod(mode)
                with self.subTest(mode=mode), self.assertRaisesRegex(module.Refusal, 'owned-ssh-known-hosts-required'):
                    module.private_ssh_options(trust)
            trust.chmod(0o600)
            link = root / 'link'
            link.symlink_to(trust)
            for path in (link, root):
                with self.subTest(path=path), self.assertRaises(module.Refusal):
                    module.private_ssh_options(path)
            with patch.object(module.os, 'getuid', return_value=os.getuid()+1):
                with self.assertRaises(module.Refusal):
                    module.private_ssh_options(trust)

    def test_controller_cutover_uses_old_image_once_and_refuses_changed_replacement(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            old, replacement = root / 'old', root / 'replacement'
            old.write_bytes(b'old controller image')
            replacement.write_bytes(b'replacement controller image')
            fixture = object.__new__(module.Fixture)
            fixture.args = SimpleNamespace(initial_webmux=old, initial_webmux_sha256=module.digest(old),
                webmux=replacement, webmux_sha256=module.digest(replacement), embedded_assets=True)
            fixture.controller, fixture.controller_starts = None, 0
            fixture.root, fixture.repo, fixture.env = root, root, {}
            fixture.address = '127.0.0.1:1'
            fixture.receipt = {'phases': [], 'controller_images': [], 'controller_asset_mode': 'embedded'}
            fixture.record_process = fixture.wait = Mock()
            fixture.epoch = lambda pid: f'epoch-{pid}'
            fixture.save = Mock()
            with (root / 'log').open('wb') as stream:
                fixture.log = lambda name: stream
                children = [Mock(pid=pid, poll=Mock(return_value=0)) for pid in (101, 102, 103)]
                with patch.object(module.subprocess, 'Popen', side_effect=children) as launch:
                    for _ in range(3):
                        fixture.start_controller()
                    self.assertEqual([call.args[0][0] for call in launch.call_args_list],
                        [str(old), str(replacement), str(replacement)])
                    self.assertTrue(all('-root' not in call.args[0] for call in launch.call_args_list))
                    replacement.write_bytes(b'changed after verification')
                    with self.assertRaisesRegex(module.Refusal, 'controller-image-changed'):
                        fixture.start_controller()
                    self.assertEqual(launch.call_count, 3)
            self.assertEqual([row['initial_image'] for row in fixture.receipt['controller_images']], [True, False, False])

    def test_default_controller_keeps_checkout_asset_override(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / 'controller'
            binary.write_bytes(b'controller image')
            fixture = object.__new__(module.Fixture)
            fixture.args = SimpleNamespace(initial_webmux=None, initial_webmux_sha256=None,
                webmux=binary, webmux_sha256=module.digest(binary), embedded_assets=False)
            fixture.controller, fixture.controller_starts = None, 0
            fixture.root, fixture.repo, fixture.env = root, root, {}
            fixture.address = '127.0.0.1:1'
            fixture.receipt = {'phases': [], 'controller_images': [], 'controller_asset_mode': 'checkout-override'}
            fixture.record_process = fixture.wait = fixture.save = Mock()
            fixture.epoch = lambda pid: f'epoch-{pid}'
            with (root / 'log').open('wb') as stream:
                fixture.log = lambda name: stream
                with patch.object(module.subprocess, 'Popen', return_value=Mock(pid=101)) as launch:
                    fixture.start_controller()
                    self.assertEqual(launch.call_args.args[0],
                        [str(binary), '-listen', fixture.address, '-root', str(root / 'webmux')])

    def test_four_independent_original_turns_must_progress(self):
        module.verify_continuity(cohort(), cohort(2))
        for key in ('session_id', 'thread_id', 'turn_id', 'viewer_epoch', 'provider_epoch', 'daemon_epoch', 'work_epoch', 'work_script_sha256', 'work_item_id'):
            after = cohort(2)
            after[2][key] += '-replacement'
            with self.subTest(key=key), self.assertRaises(module.Refusal):
                module.verify_continuity(cohort(), after)

    def test_duplicate_viewers_threads_providers_do_not_make_four_sessions(self):
        for key in ('thread_id', 'viewer_epoch', 'provider_epoch', 'work_epoch'):
            before = cohort()
            before[1][key] = before[0][key]
            with self.subTest(key=key), self.assertRaises(module.Refusal):
                module.verify_continuity(before, cohort(2))
        for count in (0, 1, 3, 5):
            with self.subTest(count=count), self.assertRaises(module.Refusal):
                module.verify_continuity((cohort()+cohort())[:count], cohort(2))

    def test_static_or_finished_turn_cannot_qualify_work(self):
        for key, value in [('work_counter', 1), ('work_counter', 0), ('work_counter', '2'), ('work_counter', True),
                           ('turn_status', 'completed'), ('management_thread_id', 'other'), ('selection_thread_id', None),
                           ('work_epoch', None), ('work_epoch', ''), ('work_script_sha256', None)]:
            after = cohort(2)
            after[1][key] = value
            with self.subTest(key=key, value=value), self.assertRaises(module.Refusal):
                module.verify_continuity(cohort(), after)

    def test_viewer_migration_requires_same_execution_and_a_new_viewer(self):
        before = cohort()[0]
        after = cohort(2)[0]
        after['viewer_epoch'] = 'replacement-viewer'
        module.verify_viewer_migration(before, after)
        with self.assertRaises(module.Refusal):
            module.verify_viewer_migration(before, cohort(2)[0])
        for key in ('turn_id', 'thread_id', 'provider_epoch', 'daemon_epoch', 'work_epoch', 'work_script_sha256', 'work_item_id'):
            changed = dict(after, **{key: 'replaced'})
            with self.subTest(key=key), self.assertRaises(module.Refusal):
                module.verify_viewer_migration(before, changed)

    def test_only_actual_command_output_of_the_original_turn_counts(self):
        marker = 'WM_CODING_nonce_'
        data = {'turns': [{'id': 'original', 'items': [
            {'type': 'userMessage', 'output': {'text': marker+'999\n'}},
            {'type': 'commandExecution', 'command': marker+'998', 'output': {'text': marker+'1\n'+marker+'2\r\n'}},
            {'type': 'commandExecution', 'output': {'text': 'echo '+marker+'997\n'}},
        ]}, {'id': 'other', 'items': [{'type': 'commandExecution', 'output': {'text': marker+'996\n'}}]}]}
        self.assertEqual(module.work_counter(data, 'original', 'nonce'), 2)
        self.assertEqual(module.work_counter(data, 'missing', 'nonce'), 0)
        self.assertEqual(module.work_counter(data, 'original', 'different'), 0)

    def test_code_mode_counts_only_native_delegated_command_results(self):
        marker = 'WM_CODING_nonce_'
        result = json.dumps({'chunk_id': 'owned', 'output': marker+'3\\n'+marker+'4\\n', 'session_id': 12})
        def data(text, name='exec', kind='functionCallOutput', namespace=None):
            return {'turns': [{'id': 'original', 'items': [{'type': kind, 'name': name,
                'namespace': namespace, 'output': {'text': text}}]}]}
        output = 'Script completed\nOutput:\n'+result
        self.assertEqual(module.work_counter(data(output), 'original', 'nonce'), 4)
        blocks = json.dumps([{'type': 'input_text', 'text': result}])
        self.assertEqual(module.work_counter(data(blocks, name='wait'), 'original', 'nonce'), 4)
        for case in [data(output, name='read_file'), data(output, kind='userMessage'),
                     data(output, namespace='other'), data(json.dumps({'last': marker+'999'})),
                     data(json.dumps({'chunk_id': 'owned', 'command': marker+'998'})),
                     data(marker+'997\n')]:
            self.assertEqual(module.work_counter(case, 'original', 'nonce'), 0)
        self.assertEqual(module.work_counter(data(output), 'other-turn', 'nonce'), 0)

    @unittest.skipUnless(sys.platform == 'darwin', 'native macOS worker inspector')
    def test_real_coding_worker_epoch_and_script_are_pinned(self):
        with tempfile.TemporaryDirectory(prefix='wm-worker-control-') as directory:
            root = Path(directory).resolve()
            project = root / 'project'
            project.mkdir()
            script = project / 'proof.py'
            script.write_text("# owned-worker-nonce\nimport time\ntime.sleep(30)\n")
            fixture = object.__new__(module.Fixture)
            fixture.root, fixture.env, fixture.receipt, fixture.streams = root, dict(os.environ), {}, []
            fixture.save = lambda: None
            fixture.record_process = lambda _role, _pid: None
            session = {'project': project, 'nonce': 'owned-worker-nonce'}
            process = subprocess.Popen([sys.executable, str(script)], cwd=project, start_new_session=True)
            try:
                self.assertEqual(fixture.coding_worker(session, 0), (None, None))
                for _ in range(20):
                    epoch, digest = fixture.coding_worker(session, 1)
                    if epoch:
                        break
                    time.sleep(.05)
                self.assertEqual(session['work_pid'], process.pid)
                self.assertEqual(fixture.coding_worker(session, 2), (epoch, digest))
                script.write_text(script.read_text()+'# changed while running\n')
                with self.assertRaises(module.Refusal):
                    fixture.coding_worker(session, 3)
                process.terminate()
                process.wait(timeout=5)
                with self.assertRaises(subprocess.CalledProcessError):
                    fixture.coding_worker(session, 4)
            finally:
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=5)
                for stream in fixture.streams:
                    stream.close()


if __name__ == '__main__':
    unittest.main()
