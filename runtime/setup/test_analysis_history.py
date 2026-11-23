import contextlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import cli


class ClearHistoryTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.runs = {
            str(index): {
                'id': str(index),
                'status': 'completed',
                'deletionScheduledFor': None,
            } for index in range(102)
        }
        self.runs['0']['status'] = 'running'
        self.runs['101'].update(status='failed', deletionScheduledFor='2030-01-01')
        self.mutations = []
        self.offsets = []
        self.reject_delete = None
        self.reject_mark = None

    def graphql(self, query, variables):
        self.assertEqual(variables['projectId'], 'target')
        if 'analysisHistory(' in query:
            offset = variables['offset']
            self.offsets.append(offset)
            return {'analysisHistory': list(self.runs.values())[offset:offset + variables['limit']]}
        run_id = variables['analysisId']
        run = self.runs[run_id]
        self.assertNotEqual(run['status'], 'running')
        if 'scheduleAnalysisDeletion(' in query:
            self.assertIsNone(run['deletionScheduledFor'])
            self.mutations.append(('mark', run_id))
            if run_id == self.reject_mark:
                return {'scheduleAnalysisDeletion': False}
            run['deletionScheduledFor'] = '2030-01-01'
            return {'scheduleAnalysisDeletion': True}
        self.assertIn('deleteMarkedAnalysis(', query)
        self.assertIsNotNone(run['deletionScheduledFor'])
        self.mutations.append(('delete', run_id))
        if run_id == self.reject_delete:
            return {'deleteMarkedAnalysis': False}
        del self.runs[run_id]
        return {'deleteMarkedAnalysis': True}

    def invoke(self, *options):
        output, errors = io.StringIO(), io.StringIO()
        with patch.object(cli.Installer, 'graphql', side_effect=self.graphql), \
                contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            code = cli.main(self.root, ['--lang', 'en', 'clear-analysis-history',
                                        '--project-id', 'target', *options])
        return code, output.getvalue(), errors.getvalue()

    def test_preview_reads_every_page_without_marking_runs(self):
        code, output, errors = self.invoke()
        self.assertEqual((code, errors), (0, ''))
        self.assertIn('target', output)
        self.assertIn('eligible for deletion: 101. Skipped: 1.', output)
        self.assertIn('--yes', output)
        self.assertEqual(self.offsets, [0, 100])
        self.assertEqual(self.mutations, [])

    def test_clear_all_pages_and_repeat_on_empty_history(self):
        code, output, errors = self.invoke('--yes')
        self.assertEqual((code, errors), (0, ''))
        self.assertIn('Confirmed deletions: 101 of 101.', output)
        self.assertEqual(list(self.runs), ['0'])
        self.assertEqual(self.mutations[-1], ('delete', '101'))
        self.runs.clear()
        self.mutations.clear()
        self.assertEqual(self.invoke('--yes')[0], 0)
        self.assertEqual(self.mutations, [])

    def test_retry_after_partial_failure_reuses_existing_mark(self):
        self.reject_delete = '2'
        code, output, errors = self.invoke('--yes')
        self.assertEqual(code, 1)
        self.assertIn('Confirmed deletions: 1 of 101.', output)
        self.assertTrue(errors)
        self.assertEqual(self.mutations, [('mark', '1'), ('delete', '1'), ('mark', '2'), ('delete', '2')])
        self.reject_delete = None
        self.assertEqual(self.invoke('--yes')[0], 0)
        self.assertEqual(list(self.runs), ['0'])

    def test_rejected_mark_stops_before_permanent_deletion(self):
        self.reject_mark = '1'
        code, output, errors = self.invoke('--yes')
        self.assertEqual(code, 1)
        self.assertIn('Confirmed deletions: 0 of 101.', output)
        self.assertTrue(errors)
        self.assertEqual(self.mutations, [('mark', '1')])
