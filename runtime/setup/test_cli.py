import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import cli


class ConfigTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.addCleanup(self.temp.cleanup)

    def test_new_config_is_private_and_stable(self):
        config = cli.Config(self.root)
        first = config.ensure('cpu')
        second = config.ensure('cpu')
        self.assertEqual(first, second)
        self.assertEqual(len(first['PROJECT_CREATOR_TOKEN']), 48)
        self.assertEqual((self.root / '.env').stat().st_mode & 0o777, 0o600)
        self.assertEqual(config.state['device'], 'cpu')

    def test_existing_values_comments_and_project_are_preserved(self):
        content = '# existing installation\nCOMPOSE_PROJECT_NAME=custom-name\nAPP_PORT=3111\nPOSTGRES_PASSWORD=existing-db\nPROJECT_CREATOR_TOKEN=existing-token-with-more-than-32-characters\nEXTRA="value with spaces"\n'
        (self.root / '.env').write_text(content)
        config = cli.Config(self.root)
        result = config.ensure('arc')
        self.assertEqual(result['POSTGRES_PASSWORD'], 'existing-db')
        self.assertEqual(config.state['project'], 'custom-name')
        self.assertTrue((self.root / '.env').read_text().startswith(content))

    def test_leading_whitespace_setting_is_replaced_once(self):
        config = cli.Config(self.root)
        config.ensure('cpu')
        path = self.root/'.env'
        path.write_text(path.read_text()+'  PROJECT_CREATOR_EMBEDDING_MODEL=bge-m3:567m\n')
        config = cli.Config(self.root)
        config.update_env({'PROJECT_CREATOR_EMBEDDING_MODEL': 'other:latest'})
        self.assertEqual(path.read_text().count('PROJECT_CREATOR_EMBEDDING_MODEL='), 1)
        self.assertEqual(config.values['PROJECT_CREATOR_EMBEDDING_MODEL'], 'other:latest')

    def test_device_switch_requires_explicit_option(self):
        config = cli.Config(self.root)
        config.ensure('arc')
        with self.assertRaisesRegex(cli.Failure, 'DEVICE_CHANGE'):
            config.ensure('cpu')
        config.ensure('cpu', switch=True)
        self.assertEqual(config.state['device'], 'cpu')

    def test_existing_unsafe_or_placeholder_secrets_fail_without_overwrite(self):
        text = 'POSTGRES_PASSWORD=replace-with-a-random-password\nPROJECT_CREATOR_TOKEN=replace-with-a-random-token-of-at-least-32-characters\n'
        (self.root / '.env').write_text(text)
        with self.assertRaisesRegex(cli.Failure, 'CONFIG_SECRET'):
            cli.Config(self.root).ensure('cpu')
        self.assertEqual((self.root / '.env').read_text(), text)

    def test_environment_project_conflict_fails(self):
        config = cli.Config(self.root)
        config.ensure('cpu', project='first')
        with self.assertRaisesRegex(cli.Failure, 'PROJECT_CHANGE'):
            config.ensure('cpu', project='second')

    def test_partial_state_is_atomically_saved(self):
        config = cli.Config(self.root)
        config.ensure('cpu')
        config.state['models'] = {'bge-m3:567m': {'digest': 'sha256:'+'a'*64}}
        config.save()
        self.assertEqual(cli.Config(self.root).state['models'], config.state['models'])


class FakeRunner:
    def __init__(self):
        self.calls = []
        self.responses = {}
        self.envs = []

    def run(self, args, *, capture=False, env=None, timeout=120, code='COMMAND', **kwargs):
        self.calls.append([str(arg) for arg in args])
        self.envs.append(env or {})
        for needle, value in self.responses.items():
            if needle in ' '.join(map(str, args)):
                if isinstance(value, Exception):
                    raise value
                return value
        return ''


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.addCleanup(self.temp.cleanup)
        for name in ['compose.yml', 'compose.intel.yml', 'compose.npu.yml']:
            (self.root / name).write_text('services: {}\n')
        self.runner = FakeRunner()
        self.app = cli.Installer(self.root, runner=self.runner, output=io.StringIO())
        self.app.config.ensure('cpu')

    def test_compose_is_explicit_preserves_project_and_drops_inherited_override(self):
        with patch.dict(os.environ, {'COMPOSE_FILE': 'evil.yml', 'PROJECT_CREATOR_TOKEN': 'do-not-inherit'}):
            self.app.compose('version')
        cmd = self.runner.calls[-1]
        self.assertIn('--project-name', cmd)
        self.assertIn(str(self.root / '.env'), cmd)
        self.assertNotIn('evil.yml', cmd)
        self.assertNotIn('COMPOSE_FILE', self.runner.envs[-1])
        self.assertNotIn('PROJECT_CREATOR_TOKEN', self.runner.envs[-1])

    def test_pull_pins_digest_and_license_and_reuses_cache(self):
        digest = 'sha256:'+'a'*64
        calls = []
        def api(path, body=None):
            calls.append((path, body))
            if path == '/api/tags':
                return {'models': [{'name': 'bge-m3:567m', 'digest': digest}]}
            return {'license': 'MIT', 'details': {'family': 'bert'}}
        self.app.ollama = api
        self.app.pin_model('bge-m3:567m', 'EMBEDDING')
        self.assertEqual(self.app.config.values['PROJECT_CREATOR_EMBEDDING_DIGEST'], digest)
        self.assertEqual(self.app.config.state['models']['bge-m3:567m']['license'], 'MIT')
        self.assertFalse(any('pull' in call for call in self.runner.calls))
        self.app.pin_model('bge-m3:567m', 'EMBEDDING')
        self.assertEqual(self.app.config.state['models']['bge-m3:567m']['digest'], digest)

    def test_missing_model_is_pulled_explicitly_then_verified(self):
        digest = 'sha256:'+'b'*64
        responses = iter([{'models': []}, {'models': [{'name': 'bge-m3:567m', 'digest': digest}]}, {'license': 'MIT'}])
        self.app.ollama = lambda *_: next(responses)
        self.app.pin_model('bge-m3:567m', 'EMBEDDING')
        self.assertTrue(any(call[-3:] == ['ollama', 'pull', 'bge-m3:567m'] for call in self.runner.calls))

    def test_tag_drift_does_not_replace_the_pinned_model(self):
        self.app.config.state['models'] = {'bge-m3:567m': {'digest': 'sha256:'+'a'*64}}
        self.app.ollama = lambda *_: {'models': [{'name': 'bge-m3:567m', 'digest': 'sha256:'+'b'*64}]}
        with self.assertRaisesRegex(cli.Failure, 'MODEL_DRIFT'):
            self.app.pin_model('bge-m3:567m', 'EMBEDDING')
        self.assertFalse(any('pull' in call for call in self.runner.calls))

    def test_existing_env_digest_is_also_a_pin(self):
        self.app.config.values['PROJECT_CREATOR_EMBEDDING_DIGEST'] = 'sha256:'+'a'*64
        self.app.ollama = lambda *_: {'models': [{'name': 'bge-m3:567m', 'digest': 'sha256:'+'b'*64}]}
        with self.assertRaisesRegex(cli.Failure, 'MODEL_DRIFT'):
            self.app.pin_model('bge-m3:567m', 'EMBEDDING')

    def test_down_never_removes_volumes_or_downloads_models(self):
        self.app.down()
        self.assertEqual(self.runner.calls[-1][-1], 'down')
        self.assertNotIn('--volumes', self.runner.calls[-1])
        self.assertFalse(any('pull' in call for call in self.runner.calls))

    def test_device_execution_checks_memory_and_digest(self):
        digest = 'sha256:'+'a'*64
        self.app.config.values['PROJECT_CREATOR_GENERATOR_MODEL'] = 'qwen2.5:7b'
        self.app.config.values['PROJECT_CREATOR_GENERATOR_DIGEST'] = digest
        self.app.ollama = lambda *_: {'models': [{'name': 'qwen2.5:7b', 'digest': digest, 'size_vram': 0}]}
        self.app.check_execution('GENERATOR')
        self.app.config.state['device'] = 'arc'
        with self.assertRaisesRegex(cli.Failure, 'DEVICE_EXECUTION'):
            self.app.check_execution('GENERATOR')
        self.app.ollama = lambda *_: {'models': [{'name': 'qwen2.5:7b', 'digest': digest, 'size_vram': 1024}]}
        self.app.check_execution('GENERATOR')

    def test_unavailable_npu_is_not_replaced_with_cpu(self):
        self.app.config.state['device'] = 'npu'
        self.app.npu_health = lambda: {'device': 'CPU', 'profile': 'a'*64}
        with self.assertRaisesRegex(cli.Failure, 'DEVICE_EXECUTION'):
            self.app.check_execution('GENERATOR')

    def test_export_uses_explicit_task_and_disk_tmpdir(self):
        self.app.config.state['source'] = {'path': str(self.root / 'source'), 'revision': 'a'*40}
        source = self.root / 'source'
        source.mkdir()
        (source / 'LICENSE').write_text('Apache-2.0')
        def fake_run(args, **kwargs):
            self.runner.calls.append(list(map(str,args)))
            self.runner.envs.append(kwargs.get('env', {}))
            if 'export' in args:
                out = Path(args[-1]); out.mkdir(parents=True, exist_ok=True)
                (out / 'openvino_model.xml').write_text('xml')
                (out / 'openvino_model.bin').write_bytes(b'bin')
                (out / 'tokenizer_config.json').write_text('{}')
            return ''
        self.runner.run = fake_run
        with patch.object(self.app, 'manifest', return_value='a'*64):
            self.app.export_npu()
        export_cmd = next(call for call in self.runner.calls if 'export' in call)
        self.assertEqual(export_cmd[export_cmd.index('--task')+1], 'text-generation-with-past')
        export_env = self.runner.envs[self.runner.calls.index(export_cmd)]
        self.assertEqual(export_env['TMPDIR'], str(self.root / '.npu' / 'export-tmp'))
        self.assertEqual(export_env['HF_HUB_OFFLINE'], '1')
        self.assertTrue((self.root / '.npu/models/qwen2.5-7b-int4/SOURCE-LICENSE').exists())


class VerifyTests(InstallTests):
    def test_quality_failure_is_not_success_and_checkpoint_is_preserved(self):
        self.app.graphql = lambda *args, **kwargs: {'startProjectAnalysis': {'status': 'completed', 'outcome': 'no_collisions_found', 'stale': False, 'errorCode': '', 'findings': [], 'sources': []}}
        self.app.fixture = lambda name, texts: ('p1', [{'id': 'a', 'version': 1, 'text': texts[0]}, {'id': 'b', 'version': 1, 'text': texts[1]}])
        self.app.check_execution = lambda *_: None
        with self.assertRaisesRegex(cli.Failure, 'MODEL_QUALITY'):
            self.app.verify()
        self.assertFalse(self.app.config.state.get('verified'))

    def test_invented_or_stale_sources_are_rejected(self):
        expected = [{'id': 'a', 'version': 1, 'text': 'a'}, {'id': 'b', 'version': 1, 'text': 'b'}]
        finding = {'leftId': 'a', 'rightId': 'alien', 'leftVersion': 1, 'rightVersion': 1, 'verdict': 'collision', 'explanation': 'x'}
        result = {'status': 'completed', 'outcome': 'possible_collision', 'stale': False, 'sources': expected, 'findings': [finding]}
        with self.assertRaisesRegex(cli.Failure, 'MODEL_SOURCES'):
            cli.validate_analysis(result, expected, 'collision')

    def test_each_case_requires_expected_finding_not_merely_outcome(self):
        sources = [{'id': 'a', 'version': 1, 'text': 'a'}, {'id': 'b', 'version': 1, 'text': 'b'}]
        result = {'status': 'completed', 'outcome': 'insufficient', 'stale': False, 'sources': sources, 'findings': []}
        with self.assertRaisesRegex(cli.Failure, 'MODEL_QUALITY'):
            cli.validate_analysis(result, sources, 'insufficient')

    def test_full_three_case_smoke_accepts_actual_outcome_contract(self):
        current = {}
        def fixture(name, texts):
            current['name'] = name
            current['sources'] = [{'id': 'a', 'version': 1, 'text': texts[0]}, {'id': 'b', 'version': 1, 'text': texts[1]}]
            return 'p1', current['sources']
        def graphql(*args):
            name = current['name']
            return {'startProjectAnalysis': {'status': 'completed', 'stale': False,
                'outcome': {'collision': 'possible_collision', 'compatible': 'no_collisions_found', 'insufficient': 'insufficient'}[name],
                'sources': current['sources'], 'findings': [{'leftId': 'a', 'rightId': 'b', 'leftVersion': 1,
                    'rightVersion': 1, 'verdict': name, 'explanation': 'Synthetic explanation.'}]}}
        self.app.fixture = fixture
        self.app.graphql = graphql
        self.app.check_execution = lambda *_: None
        self.app.verify()
        self.assertTrue(self.app.config.state['verified'])
        self.assertEqual(set(self.app.config.state['verification']), {'collision', 'compatible', 'insufficient'})

    def test_reused_index_explicitly_loads_embedder_before_device_check(self):
        self.app.ollama = lambda *_: {'embeddings': [[0.1]*1024]}
        calls = []
        self.app.check_execution = lambda role: calls.append(role)
        self.app.probe_embedding()
        self.assertEqual(calls, ['EMBEDDING'])

    def test_invalid_embedding_dimension_is_rejected(self):
        self.app.ollama = lambda *_: {'embeddings': [[0.1]*10]}
        with self.assertRaisesRegex(cli.Failure, 'MODEL_EMBEDDING'):
            self.app.probe_embedding()

    def test_interrupted_project_creation_is_discovered_before_retry(self):
        self.app.config.state['fixtures'] = {'collision': {'marker': '12345678'}}
        texts = ['one', 'two']
        requirements = [{'id': 'a', 'version': 1, 'text': texts[0]}, {'id': 'b', 'version': 1, 'text': texts[1]}]
        def graphql(query, variables=None):
            if 'projects(' in query:
                return {'projects': [{'id': 'restored', 'name': 'Installation diagnostic: collision [12345678]'}]}
            if 'requirements(' in query:
                return {'requirements': requirements}
            if 'indexRequirement(' in query:
                return {'indexRequirement': {'status': 'ready', 'version': 1, 'errorCode': ''}}
            raise AssertionError('Must not create another project: '+query)
        self.app.graphql = graphql
        self.app.probe_embedding = lambda: None
        project, actual = self.app.fixture('collision', texts)
        self.assertEqual(project, 'restored')
        self.assertEqual(actual, requirements)

    def test_owned_stale_socket_is_recovered_after_managed_process_dies(self):
        sock = self.app.npu/'run/runtime.sock'
        sock.parent.mkdir(parents=True)
        sock.write_text('test socket placeholder')
        stamp = {'device': 1, 'inode': 2, 'uid': os.getuid()}
        self.app.socket_stamp = lambda: stamp
        self.app.process_identity = lambda *args: False
        self.app.npu_health = lambda: (_ for _ in ()).throw(cli.Failure('NPU_SOCKET', 'connection refused'))
        self.app.recover_socket({'pid': 1234, 'socket': stamp})
        self.assertFalse(sock.exists())

    def test_changed_or_live_socket_is_never_removed(self):
        sock = self.app.npu/'run/runtime.sock'
        sock.parent.mkdir(parents=True)
        sock.write_text('test socket placeholder')
        self.app.socket_stamp = lambda: {'device': 1, 'inode': 999, 'uid': os.getuid()}
        self.app.process_identity = lambda *args: False
        with self.assertRaisesRegex(cli.Failure, 'NPU_SOCKET'):
            self.app.recover_socket({'pid': 1234, 'socket': {'device': 1, 'inode': 2, 'uid': os.getuid()}})
        self.assertTrue(sock.exists())

    def test_process_stamp_mismatch_blocks_pid_reuse(self):
        record = {'pid': 1234, 'start': '111', 'boot': 'boot-one'}
        self.app.process_stamp = lambda pid: {'pid': 1234, 'start': '222', 'boot': 'boot-one'}
        raw = str(self.root/'runtime/npu/npu_runtime.py').encode()+b'\0serve\0'
        with patch.object(Path, 'read_bytes', return_value=raw):
            self.assertFalse(self.app.process_identity(1234, record))


class UtilityTests(unittest.TestCase):
    def test_runner_does_not_leak_captured_output_on_failure(self):
        with self.assertRaises(cli.Failure) as caught:
            cli.Runner().run(['python3', '-c', 'print("SECRET"); raise SystemExit(7)'], capture=True)
        self.assertNotIn('SECRET', str(caught.exception))

    def test_catalogs_have_matching_keys_and_arguments(self):
        self.assertEqual(cli.MESSAGES['ru'].keys(), cli.MESSAGES['en'].keys())
        import string
        for key in cli.MESSAGES['ru']:
            params = lambda lang: {name for _, name, _, _ in string.Formatter().parse(cli.MESSAGES[lang][key]) if name}
            self.assertEqual(params('ru'), params('en'), key)


if __name__ == '__main__':
    unittest.main()
