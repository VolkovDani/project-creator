"""Offline checks for the single-record classification experiment."""
import importlib.util
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import subprocess
import sys
from tempfile import TemporaryDirectory
from threading import Thread
from pathlib import Path
import unittest

import classification_benchmark as bench

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location('evaluator', ROOT / 'scripts/classification-evaluator.py')
evaluator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(evaluator)


class BenchmarkTest(unittest.TestCase):
    def setUp(self):
        self.contract = json.loads((ROOT / 'contracts/classification.json').read_text())
        self.corpus = {'status': 'reviewed', 'language': 'ru', 'taxonomy': self.contract['version'], 'reviewer': 'test reviewer',
                       'reviewed_at': '2026-11-20', 'examples': [
            {'id': key, 'text': text, 'source': 'https://example.test/' + key,
             'license': 'CC0-1.0', 'split': split, 'semantic': 'functional_requirement',
             'representation': 'statement'} for key, text, split in [
                ('t', 'Сохранить запись.', 'tuning'),
                ('h', 'Ввод. ' * 150 + 'Дано: очередь пуста.', 'holdout')]]}

    def test_review_and_contract_checked_before_any_inference(self):
        for change in ({'status': 'proposed'}, {'taxonomy': 'stale'}, {'reviewer': ''}, {'reviewed_at': 'yesterday'}):
            with self.subTest(change=change), self.assertRaises(ValueError):
                bench.validate_corpus(self.corpus | change, self.contract)
        for change in ({'proposed': {}}, {'source': ''}, {'semantic': 'invented'}, {'id': 't'},
                       {'text': self.corpus['examples'][0]['text']}, {'split': 'tuning'}):
            rows = self.corpus['examples']
            with self.subTest(change=change), self.assertRaises(ValueError):
                bench.validate_corpus(self.corpus | {'examples': [rows[0], rows[1] | change]}, self.contract)
        bench.validate_corpus(self.corpus, self.contract)

    def test_full_and_marker_record_exact_offsets_and_never_clip_full_input(self):
        text = self.corpus['examples'][1]['text']
        state, coverage = bench.select_input(text, 'full', 512, 96)
        self.assertIsNone(state)
        self.assertEqual(coverage['reason'], 'over_budget')
        state, coverage = bench.select_input(text, 'marker', 512, 96)
        self.assertEqual(state, 'Дано: очередь пуста.')
        self.assertEqual(state, text[coverage['start']:coverage['end']])
        self.assertEqual(coverage['input_bytes'], len(state.encode()))
        self.assertIsNone(bench.select_input('слово ' * 200, 'marker', 65536, 96)[0])
        state, cov = bench.select_input('Дано ' + '"\\я' * 1000, 'marker', 65536, 96)
        self.assertLessEqual(cov['state_bytes'], 96)
        self.assertEqual(bench.select_input(text, 'full', 65536, 96)[0], text)
        self.assertGreater(cov['end'], 1)

    def test_schemes_separate_effects_and_store_diagnostics(self):
        calls = []
        def ask(state, question):
            calls.append((state, question))
            options = list(question['criteria'])
            scores = dict.fromkeys(options, 0.)
            scores[options[0]] = 1.
            return {'choice': options[0], 'probabilities': scores, 'confidence': 1.,
                    'type': 'choice'}, {'input_tokens': 12, 'output_tokens': 1}
        results = bench.run_experiments(self.corpus, self.contract, ask, 512, 96)
        schemes = results['schemes']
        self.assertEqual(len(schemes), 6)
        base, long = schemes['flat']['predictions']
        self.assertEqual(base['semantic']['label'], self.contract['semantic'][0])
        self.assertIsNone(long['semantic']['label'])
        self.assertEqual(schemes['flat_reverse']['predictions'][0]['semantic']['label'], self.contract['semantic'][-1])
        self.assertEqual(len(schemes['hierarchical']['predictions'][0]['semantic']['steps']), 2)
        self.assertIn('user_story_probe', schemes['story_probe']['predictions'][0])
        self.assertEqual(schemes['flat_marker']['predictions'][1]['input']['input_bytes'], len('Дано: очередь пуста.'.encode()))
        self.assertTrue(all(state in (self.corpus['examples'][0]['text'], 'Дано: очередь пуста.') for state, _ in calls))
        self.assertTrue(results['questions'])

    def test_runtime_limit_is_separate_from_a_model_choice_and_other_axes_complete(self):
        def ask(state, question):
            if len(question['criteria']) > 6:
                raise bench.RequestRejected(400)
            option = next(iter(question['criteria']))
            scores = {key: float(key == option) for key in question['criteria']}
            return {'type': 'choice', 'choice': option, 'probabilities': scores, 'confidence': 1}, {
                'input_tokens': 1200 if len(state) > 300 else 30, 'output_tokens': 1}
        experiment = bench.run_experiments(self.corpus, self.contract, ask, 65536, 96)
        rejected = experiment['schemes']['flat']['predictions'][0]['semantic']
        self.assertEqual(rejected['reason'], 'runtime_rejected')
        self.assertEqual(rejected['steps'][0]['http_status'], 400)
        long = experiment['schemes']['story_probe']['predictions'][1][bench.PROBE]
        self.assertEqual(long['reason'], 'outside_training_context')
        self.assertIsNone(long['label'])
        self.assertEqual(long['steps'][0]['usage']['input_tokens'], 1200)
        self.assertIn('scores', long['steps'][0])
        self.assertIsNotNone(experiment['schemes']['story_probe']['predictions'][0][bench.PROBE]['label'])
        for head in experiment['questions'].values():
            self.assertLessEqual(head['head_bounds']['all_tokens_upper_bound'], 256)
            self.assertLessEqual(head['head_bounds']['longest_option_tokens_upper_bound'], 48)

    def test_pin_and_answer_fail_closed(self):
        digest = 'a' * 64
        model = {'name': bench.MODEL, 'digest': 'sha256:' + digest}
        bench.check_pin({'models': [model]}, digest)
        for bad in ({'digest': 'b' * 64}, {'remote_model': 'x'}, {'remote_host': 'x'}, {'name': 'laya:latest'}):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                bench.check_pin({'models': [model | bad]}, digest)
        answer = {'choice': 'a', 'probabilities': {'a': .8, 'b': .2}, 'type': 'choice', 'confidence': .8}
        response = {'model': bench.MODEL, 'answers': {'classification': answer},
                    'usage': {'input_tokens': 8, 'output_tokens': 1}}
        self.assertEqual(bench.decode(response, ['a', 'b'])[0], answer)
        for bad in ({'choice': 'x'}, {'probabilities': {'a': True, 'b': False}},
                    {'probabilities': {'a': .2, 'b': .2}}, {'confidence': float('nan')}):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                bench.decode(response | {'answers': {'classification': answer | bad}}, ['a', 'b'])

    def test_cli_records_loopback_stub_and_verifies_each_request(self):
        digest = 'c1242e4a83f0e1cb58f0f191a65bbc5d3306fae274d152b8229ec574999d465d'
        calls = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def reply(self, value):
                self.send_response(200)
                self.end_headers()
                self.wfile.write(json.dumps(value).encode())

            def do_GET(self):
                calls.append(self.path)
                if self.path == '/api/version':
                    self.reply({'version': '0.40.16'})
                else:
                    self.reply({'models': [{'name': bench.MODEL, 'digest': digest}]})

            def do_POST(self):
                calls.append(self.path)
                if self.path == '/api/show':
                    self.reply({'details': {'format': 'gguf', 'runner': 'llamacpp', 'family': 'modern-bert',
                                            'quantization_level': 'BF16'},
                                'capabilities': ['decision'], 'model_info': {
                                    'general.architecture': 'modern-bert',
                                    'modern-bert.decision.type': 'laya',
                                    'modern-bert.decision.max_head_tokens': 256}})
                    return
                query = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                question = query['questions']['classification']
                options = list(question['criteria'])
                scores = {key: float(key == options[0]) for key in options}
                self.reply({'model': query['model'], 'answers': {'classification': {
                    'type': 'choice', 'choice': options[0], 'probabilities': scores, 'confidence': 1}},
                    'usage': {'input_tokens': 12, 'output_tokens': 1}})

        with HTTPServer(('127.0.0.1', 0), Handler) as server, TemporaryDirectory() as temp:
            thread = Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                path = Path(temp, 'frozen.json')
                path.write_text(json.dumps(self.corpus), encoding='utf-8')
                result = subprocess.run([sys.executable, '-B', str(ROOT / 'scripts/classification-evaluator.py'),
                    '--run-local', '--examples', str(path), '--contract', str(ROOT / 'contracts/classification.json'),
                    '--port', str(server.server_port), '--digest', digest, '--target-precision', '.9',
                    '--min-accepted', '1', '--full-state-bytes', '512'], capture_output=True, text=True, env={'PATH': ''})
                self.assertEqual(result.returncode, 0, result.stderr)
                report = json.loads(result.stdout)
                self.assertEqual(report['provenance']['ollama_version'], '0.40.16')
                self.assertEqual(report['provenance']['digest'], 'sha256:' + digest)
                self.assertEqual(report['results']['flat']['semantic']['holdout']['unavailable'], ['h'])
                self.assertIsNotNone(report['provenance']['examples_sha256'])
                paired = report['view_comparison']['holdout']['semantic']
                self.assertEqual(paired['records'], 1)
                self.assertEqual(paired['longer_than_marker_window'], 1)
                self.assertEqual(paired['full_available'], 0)
                self.assertEqual(paired['marker_available'], 1)
                self.assertEqual(paired['paired_ids'], [])
                for i, call in enumerate(calls):
                    if call == '/v1/systemone':
                        self.assertEqual(calls[i - 1], '/api/tags')
                        self.assertEqual(calls[i + 1], '/api/tags')
            finally:
                server.shutdown()
                thread.join()

    def test_no_evidence_below_minimum_tuning_score_means_review(self):
        data = [('t', 'x', 'x', .8, 'tuning'), ('h', 'x', 'x', .4, 'holdout')]
        result = evaluator.threshold_report(data, ['x', 'y'], .8, 1)
        self.assertEqual(result['threshold'], .8)
        self.assertEqual(result['holdout']['below_threshold'], ['h'])

    def test_threshold_freezes_on_tuning_and_keeps_holdout_abstentions(self):
        data = [('a', 'x', 'x', .9, 'tuning'), ('b', 'x', 'y', .5, 'tuning'),
                ('c', 'x', 'y', .95, 'holdout'), ('d', 'x', 'x', .2, 'holdout'),
                ('e', 'x', None, None, 'holdout')]
        result = evaluator.threshold_report(data, ['x', 'y'], 1., 1)
        self.assertEqual(result['threshold'], .9)
        holdout = result['holdout']
        self.assertEqual(holdout['metrics']['coverage'], 1/3)
        self.assertEqual(holdout['accepted_errors'], ['c'])
        self.assertEqual(holdout['below_threshold'], ['d'])
        self.assertEqual(holdout['unavailable'], ['e'])
        self.assertIsNone(evaluator.threshold_report(data, ['x', 'y'], 1., 2)['threshold'])


if __name__ == '__main__':
    unittest.main()
