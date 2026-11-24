"""Receipt refusal controls only; these do not qualify native continuity."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('four_codex', Path(__file__).with_name('qualify-four-codex-sessions.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def cohort(counter=1):
    return [dict(session_id=f'session-{i}', thread_id=f'thread-{i}', turn_id=f'turn-{i}',
        viewer_epoch=f'viewer-{i}', provider_epoch=f'provider-{i}', daemon_epoch='daemon',
        turn_status='inProgress', management_thread_id=f'thread-{i}', selection_thread_id=f'thread-{i}',
        work_counter=counter) for i in range(4)]


class ContinuityControls(unittest.TestCase):
    def test_four_independent_original_turns_must_progress(self):
        module.verify_continuity(cohort(), cohort(2))
        for key in ('session_id', 'thread_id', 'turn_id', 'viewer_epoch', 'provider_epoch', 'daemon_epoch'):
            after = cohort(2)
            after[2][key] += '-replacement'
            with self.subTest(key=key), self.assertRaises(module.Refusal):
                module.verify_continuity(cohort(), after)

    def test_duplicate_viewers_threads_providers_do_not_make_four_sessions(self):
        for key in ('thread_id', 'viewer_epoch', 'provider_epoch'):
            before = cohort()
            before[1][key] = before[0][key]
            with self.subTest(key=key), self.assertRaises(module.Refusal):
                module.verify_continuity(before, cohort(2))
        for count in (0, 1, 3, 5):
            with self.subTest(count=count), self.assertRaises(module.Refusal):
                module.verify_continuity((cohort()+cohort())[:count], cohort(2))

    def test_static_or_finished_turn_cannot_qualify_work(self):
        for key, value in [('work_counter', 1), ('work_counter', 0), ('work_counter', '2'), ('work_counter', True),
                           ('turn_status', 'completed'), ('management_thread_id', 'other'), ('selection_thread_id', None)]:
            after = cohort(2)
            after[1][key] = value
            with self.subTest(key=key, value=value), self.assertRaises(module.Refusal):
                module.verify_continuity(cohort(), after)

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


if __name__ == '__main__':
    unittest.main()
