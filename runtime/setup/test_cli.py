import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
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

    def test_legacy_npu_state_defaults_ollama_device_to_cpu(self):
        config = cli.Config(self.root)
        config.ensure('npu')
        self.assertEqual(config.state.get('ollama_device', 'cpu'), 'cpu')

    def test_npu_can_persist_arc_ollama_device_and_requires_explicit_switch(self):
        config = cli.Config(self.root)
        config.ensure('npu', ollama_device='arc')
        self.assertEqual(config.state['device'], 'npu')
        self.assertEqual(config.state['ollama_device'], 'arc')
        with self.assertRaisesRegex(cli.Failure, 'DEVICE_CHANGE'):
            config.ensure('npu', ollama_device='cpu')
        config.ensure('npu', ollama_device='cpu', switch=True)


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


class InstallerTestCase(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.addCleanup(self.temp.cleanup)
        for name in ['compose.yml', 'compose.intel.yml', 'compose.npu.yml']:
            (self.root / name).write_text('services: {}\n')
        self.runner = FakeRunner()
        self.app = cli.Installer(self.root, runner=self.runner, output=io.StringIO())
        self.app.config.ensure('cpu')


class InstallTests(InstallerTestCase):
    @unittest.skipUnless(cli.shutil.which('docker'), 'Docker Compose is required')
    def test_compose_passes_analysis_threshold_from_dotenv_to_backend(self):
        key = 'PROJECT_CREATOR_ANALYSIS_DEFAULT_THRESHOLD'
        self.app.config.update_env({key: '0.8'})
        repo = Path(cli.__file__).resolve().parents[2]
        result = subprocess.run(
            ['docker', 'compose', '--env-file', str(self.root/'.env'), '-f', str(repo/'compose.yml'),
             'config', '--format', 'json'],
            capture_output=True, text=True, check=True, env=self.app.environment())
        services = json.loads(result.stdout)['services']
        self.assertEqual(services['backend']['environment'].get(key), '0.8')

    def test_compose_is_explicit_preserves_project_and_drops_inherited_override(self):
        with patch.dict(os.environ, {'COMPOSE_FILE': 'evil.yml', 'PROJECT_CREATOR_TOKEN': 'do-not-inherit'}):
            self.app.compose('version')
        cmd = self.runner.calls[-1]
        self.assertIn('--project-name', cmd)
        self.assertIn(str(self.root / '.env'), cmd)
        self.assertNotIn('evil.yml', cmd)
        self.assertNotIn('COMPOSE_FILE', self.runner.envs[-1])
        self.assertNotIn('PROJECT_CREATOR_TOKEN', self.runner.envs[-1])

    def test_npu_arc_compose_includes_intel_then_npu_overrides(self):
        self.app.config.ensure('npu', ollama_device='arc', switch=True)
        self.app.compose('config')
        command = self.runner.calls[-1]
        files = [command[i + 1] for i, value in enumerate(command[:-1]) if value == '-f']
        self.assertEqual(files, [str(self.root / name) for name in ('compose.yml', 'compose.intel.yml', 'compose.npu.yml')])

    def test_invalid_saved_ollama_device_is_rejected_without_cpu_fallback(self):
        self.app.config.state.update(device='npu', ollama_device='invalid')
        with self.assertRaisesRegex(cli.Failure, 'CONFIG_STATE'):
            self.app.compose('config')

    def test_embedding_execution_checks_saved_ollama_device_for_npu_arc(self):
        self.app.config.ensure('npu', ollama_device='arc', switch=True)
        self.app.config.values.update({'PROJECT_CREATOR_EMBEDDING_MODEL': 'bge-m3:567m', 'PROJECT_CREATOR_EMBEDDING_DIGEST': 'sha256:'+'a'*64})
        self.app.ollama = lambda path, body=None: {'models': [{'name': 'bge-m3:567m', 'digest': 'sha256:'+'a'*64, 'size_vram': 10, 'size': 100}]} if path == '/api/ps' else {}
        self.app.check_execution('EMBEDDING')
        self.assertEqual(self.app.config.state['execution']['EMBEDDING']['device'], 'arc')

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

    def test_unprefixed_api_digest_matches_existing_pin_through_setup_up_and_doctor(self):
        digest = 'sha256:'+'a'*64
        self.app.config.update_env(dict(PROJECT_CREATOR_EMBEDDING_MODEL='bge-m3:567m',
                                      PROJECT_CREATOR_EMBEDDING_DIGEST=digest,
                                      PROJECT_CREATOR_GENERATOR_MODEL='qwen2.5:7b',
                                      PROJECT_CREATOR_GENERATOR_DIGEST=digest))
        self.app.ollama = lambda path, body=None: (
            {'license': 'MIT'} if path == '/api/show' else
            {'models': [{'name': name, 'digest': 'a'*64, 'size_vram': 0}
                        for name in ('bge-m3:567m', 'qwen2.5:7b')]})
        self.app.pin_model('bge-m3:567m', 'EMBEDDING')
        self.assertEqual(self.app.config.values['PROJECT_CREATOR_EMBEDDING_DIGEST'], digest)
        self.app.check_execution('EMBEDDING')
        with patch.object(self.app, 'prerequisites'), patch.object(self.app, 'check_ui'):
            self.app.doctor()

    def test_short_or_invalid_api_digest_is_rejected(self):
        for digest in ('a'*12, 'sha256:'+'g'*64, None, 123):
            with self.subTest(digest=digest):
                self.app.ollama = lambda *_: {'models': [{'name': 'bge-m3:567m', 'digest': digest}]}
                with self.assertRaisesRegex(cli.Failure, 'MODEL_DIGEST'):
                    self.app.pin_model('bge-m3:567m', 'EMBEDDING')

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


class LogTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.addCleanup(self.temp.cleanup)
        self.runner = FakeRunner()
        self.app = cli.Installer(self.root, runner=self.runner, output=io.StringIO())
        self.app.config.ensure('npu')

    def test_runtime_stdout_and_stderr_share_private_log_under_logs(self):
        profile = 'a'*64
        cli.atomic(self.app.npu/'profile.sha256', profile+'\n')
        def start(arguments, **kwargs):
            self.assertIs(kwargs['stdout'], kwargs['stderr'])
            kwargs['stdout'].write('runtime output\n')
            return SimpleNamespace(pid=1234, poll=lambda: None)
        with patch.object(cli.subprocess, 'Popen', side_effect=start), \
             patch.object(self.app, 'process_stamp', return_value={'pid': 1234, 'start': '1', 'boot': 'test'}), \
             patch.object(self.app, 'npu_health', return_value={'device': 'NPU', 'profile': profile}):
            self.app.start_npu()
        logfile = self.root/'logs/npu/runtime.log'
        self.assertEqual(logfile.read_text(), 'runtime output\n')
        self.assertEqual(logfile.stat().st_mode & 0o777, 0o600)
        self.assertEqual(logfile.parent.stat().st_mode & 0o777, 0o700)
        self.assertFalse((self.app.npu/'runtime.log').exists())

    def test_uv_install_reports_go_to_logs_and_freeze_remains_model_provenance(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        (requirements/'requirements-export.txt').write_text('example==2.0\n')
        def run(arguments, **kwargs):
            arguments = list(map(str, arguments))
            self.runner.calls.append(arguments)
            if arguments[1:3] == ['--version']:
                return 'uv 0.12.13\n'
            if 'venv' in arguments and '--python' in arguments:
                folder = Path(arguments[-1])
                (folder/'bin').mkdir(parents=True, exist_ok=True)
                (folder/'bin/python').touch()
                return ''
            if arguments[1:3] == ['pip', 'freeze']:
                environment_python = arguments[arguments.index('--python')+1]
                return 'example==2.0\n' if 'export-env' in environment_python else 'example==1.0\n'
            return ''
        self.runner.run = run
        with patch.object(cli.shutil, 'which', return_value='/usr/bin/uv'), \
             patch.object(self.app, 'python_npu', return_value='python3.12'):
            self.app.environments_npu()
        installs = [call for call in self.runner.calls if call[1:3] == ['pip', 'install']]
        self.assertEqual(len(installs), 2)
        self.assertTrue(all('--python' in call for call in installs))
        reports = list((self.root/'logs/npu').glob('*-install.json'))
        self.assertEqual(len(reports), 2)
        for report in reports:
            data = json.loads(report.read_text())
            self.assertEqual(data['installer'], 'uv')
            self.assertEqual(data['report_format'], 'project-creator.uv-install-diagnostics.v1')
            self.assertEqual(data['status'], 'installed')
            requirement_text = 'example==2.0\n' if data['requirements_file'].endswith('requirements-export.txt') else 'example==1.0\n'
            self.assertEqual(data['requirements_sha256'], cli.hashlib.sha256(requirement_text.encode()).hexdigest())
            env_name = 'export-env' if data['requirements_file'].endswith('requirements-export.txt') else 'runtime-env'
            marker_data = json.loads((self.app.npu/env_name/'project-creator-requirements.sha256').read_text())
            self.assertEqual(marker_data['requirements_sha256'], data['requirements_sha256'])
        self.assertFalse(any('ensurepip' in call or call[:2] == ['python3.12', '-m'] for call in self.runner.calls))
        self.assertEqual((self.app.npu/'export-env-freeze.txt').read_text(), 'example==2.0\n')
        self.assertEqual((self.app.npu/'runtime-env-freeze.txt').read_text(), 'example==1.0\n')

    def test_missing_environment_is_reinstalled_even_when_marker_matches(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        folder = self.app.npu/'runtime-env'
        folder.mkdir(parents=True)
        digest = cli.hashlib.sha256(b'example==1.0\n').hexdigest()
        (folder/'project-creator-requirements.sha256').write_text(json.dumps({
            'schema_version': 1, 'installer': 'uv', 'requirements_sha256': digest,
        })+'\n')
        def run(arguments, **kwargs):
            arguments = list(map(str, arguments)); self.runner.calls.append(arguments)
            if arguments[1:3] == ['--version']:
                return 'uv 0.12.13\n'
            if 'venv' in arguments and '--python' in arguments:
                environment = Path(arguments[-1]); (environment/'bin').mkdir(parents=True); (environment/'bin/python').touch()
            if arguments[1:3] == ['pip', 'freeze']:
                return 'example==1.0\n'
            return ''
        self.runner.run = run
        with patch.object(cli.shutil, 'which', return_value='/usr/bin/uv'), \
             patch.object(self.app, 'python_npu', return_value='python3.12'):
            self.app.environments_npu(prepare_export=False)
        self.assertTrue(any(call[1:3] == ['pip', 'install'] for call in self.runner.calls))
        venv = next(call for call in self.runner.calls if 'venv' in call)
        self.assertIn('--allow-existing', venv, 'uv must preserve marker-only partial directories during recovery')

    def test_unchanged_requirements_skip_uv_install_and_legacy_marker_is_upgraded(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        folder = self.app.npu/'runtime-env'
        (folder/'bin').mkdir(parents=True)
        (folder/'bin/python').touch()
        marker = folder/'project-creator-requirements.sha256'
        digest = cli.hashlib.sha256(b'example==1.0\n').hexdigest()
        marker.write_text(digest+'\n')
        calls = []
        def run(arguments, **kwargs):
            arguments = list(map(str, arguments)); calls.append(arguments)
            if arguments[1:3] == ['--version']:
                return 'uv 0.12.13\n'
            return 'example==1.0\n' if arguments[1:3] == ['pip', 'freeze'] else ''
        self.runner.run = run
        with patch.object(cli.shutil, 'which', return_value='/usr/bin/uv'), \
             patch.object(self.app, 'python_npu', return_value='python3.12'):
            self.app.environments_npu(prepare_export=False)
            self.app.environments_npu(prepare_export=False)
        installs = [call for call in calls if call[1:3] == ['pip', 'install']]
        self.assertEqual(len(installs), 1, 'legacy pip marker should trigger one uv migration; next run skips')
        marker_data = json.loads(marker.read_text())
        self.assertEqual(marker_data, {'schema_version': 1, 'installer': 'uv', 'requirements_sha256': digest})
        self.assertFalse(any('venv' in call and '--python' in call for call in calls))

    def test_existing_environment_without_pip_is_reused_without_ensurepip(self):
        folder = self.app.npu/'runtime-env'
        subprocess.run([sys.executable, '-m', 'venv', '--without-pip', folder], check=True,
                       capture_output=True)
        python = str(folder/'bin/python')
        probe = [python, '-c', 'import pip']
        self.assertNotEqual(subprocess.run(probe, capture_output=True).returncode, 0)
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        def run(arguments, **kwargs):
            arguments = list(map(str, arguments))
            self.runner.calls.append(arguments)
            if arguments[1:3] == ['--version']:
                return 'uv 0.12.13\n'
            if arguments[1:3] == ['pip', 'freeze']:
                return 'example==1.0\n'
            return ''
        self.runner.run = run
        with patch.object(cli.shutil, 'which', return_value='/usr/bin/uv'), \
             patch.object(self.app, 'python_npu', return_value=sys.executable):
            self.app.environments_npu(prepare_export=False)
        self.assertFalse(any('ensurepip' in call for call in self.runner.calls))
        self.assertNotEqual(subprocess.run(probe, capture_output=True).returncode, 0)
        install = next(call for call in self.runner.calls if call[1:3] == ['pip', 'install'])
        self.assertEqual(install[install.index('--python')+1], python)

    def test_missing_uv_is_actionable_and_does_not_create_environment(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        with patch.object(cli.shutil, 'which', return_value=None):
            with self.assertRaisesRegex(cli.Failure, 'UV_MISSING'):
                self.app.environments_npu(prepare_export=False)
        self.assertFalse((self.app.npu/'runtime-env').exists())

    def test_unchanged_uv_checkpoint_does_not_require_uv(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        folder = self.app.npu/'runtime-env'
        (folder/'bin').mkdir(parents=True)
        (folder/'bin/python').touch()
        digest = cli.hashlib.sha256(b'example==1.0\n').hexdigest()
        (folder/'project-creator-requirements.sha256').write_text(json.dumps({
            'schema_version': 1, 'installer': 'uv', 'requirements_sha256': digest,
        })+'\n')
        with patch.object(cli.shutil, 'which', return_value=None):
            self.app.environments_npu(prepare_export=False)
        self.assertFalse(any('uv' in call for call in self.runner.calls))

    def test_uv_install_failure_writes_diagnostic_but_no_marker_or_freeze(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        def run(arguments, **kwargs):
            arguments = list(map(str, arguments)); self.runner.calls.append(arguments)
            if arguments[1:3] == ['--version']:
                return 'uv 0.12.13\n'
            if 'venv' in arguments and '--python' in arguments:
                folder = Path(arguments[-1]); (folder/'bin').mkdir(parents=True, exist_ok=True); (folder/'bin/python').touch()
                return ''
            if arguments[1:3] == ['pip', 'install']:
                raise cli.Failure('PYTHON_INSTALL', 'uv exited with status 1.')
            return ''
        self.runner.run = run
        with patch.object(cli.shutil, 'which', return_value='/usr/bin/uv'), \
             patch.object(self.app, 'python_npu', return_value='python3.12'):
            with self.assertRaisesRegex(cli.Failure, 'PYTHON_INSTALL'):
                self.app.environments_npu(prepare_export=False)
        folder = self.app.npu/'runtime-env'
        self.assertFalse((folder/'project-creator-requirements.sha256').exists())
        self.assertFalse((self.app.npu/'runtime-env-freeze.txt').exists())
        report = json.loads((self.root/'logs/npu/runtime-env-install.json').read_text())
        self.assertEqual(report['status'], 'failed')

    def test_unsupported_uv_version_is_rejected(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        self.runner.responses['--version'] = 'uv 0.12.12\n'
        with patch.object(cli.shutil, 'which', return_value='/usr/bin/uv'):
            with self.assertRaisesRegex(cli.Failure, 'UV_VERSION'):
                self.app.environments_npu(prepare_export=False)

    def test_openvino_and_huggingface_telemetry_controls_remain_explicit(self):
        requirements = self.root/'runtime/npu'
        requirements.mkdir(parents=True)
        (requirements/'requirements.txt').write_text('example==1.0\n')
        def run(arguments, **kwargs):
            arguments=list(map(str,arguments)); self.runner.calls.append(arguments); self.runner.envs.append(kwargs.get('env') or {})
            if arguments[1:3] == ['--version']:
                return 'uv 0.12.13\n'
            if 'venv' in arguments and '--python' in arguments:
                folder=Path(arguments[-1]); (folder/'bin').mkdir(parents=True,exist_ok=True); (folder/'bin/python').touch()
            if arguments[1:3] == ['pip', 'freeze']:
                return 'example==1.0\n'
            return ''
        self.runner.run=run
        with patch.object(cli.shutil,'which',return_value='/usr/bin/uv'), \
             patch.object(self.app,'python_npu',return_value='python3.12'):
            self.app.environments_npu(prepare_export=False)
        self.assertTrue(any(Path(call[0]).name == 'opt_in_out' and call[-1] == '--opt_out'
                            for call in self.runner.calls))
        install_index=next(i for i,call in enumerate(self.runner.calls) if call[1:3]==['pip','install'])
        environment=self.runner.envs[install_index]
        self.assertEqual(environment['HF_HUB_DISABLE_TELEMETRY'],'1')
        self.assertEqual(environment['HF_HUB_DISABLE_IMPLICIT_TOKEN'],'1')
        self.assertEqual(environment['UV_CACHE_DIR'], str(self.app.npu/'uv-cache'))
        self.assertEqual(environment['UV_NO_PROGRESS'],'1')


class RemoteInstallTests(InstallerTestCase):
    def configure_remote(self, *roles):
        for role in roles:
            prefix = 'PROJECT_CREATOR_'+role
            self.app.config.update_env({prefix+'_URL': 'http://inference.local/v1',
                                        prefix+'_MODEL': role.lower()+'-model',
                                        prefix+'_API_KEY': 'private-'+role})

    @unittest.skipUnless(cli.shutil.which('docker'), 'Docker Compose is required')
    def test_remote_compose_renders_only_app_services_and_scopes_credentials_to_backend(self):
        self.configure_remote('GENERATOR', 'EMBEDDING')
        self.app.config.update_env({'PROJECT_CREATOR_EMBEDDING_DIMENSIONS': '1024'})
        repo = Path(cli.__file__).resolve().parents[2]
        args = ['docker', 'compose', '--env-file', str(self.root/'.env'), '-f', str(repo/'compose.yml'),
                '-f', str(repo/'compose.remote.yml'), 'config', '--format', 'json']
        result = subprocess.run(args, capture_output=True, text=True, env=self.app.environment())
        self.assertEqual(result.returncode, 0, result.stderr)
        services = json.loads(result.stdout)['services']
        self.assertEqual(set(services), {'frontend', 'backend', 'postgres'})
        settings = services['backend']['environment']
        for role in ('GENERATOR', 'EMBEDDING'):
            prefix = 'PROJECT_CREATOR_'+role
            self.assertEqual(settings[prefix+'_RUNTIME'], 'openai-compatible')
            self.assertEqual(settings[prefix+'_URL'], 'http://inference.local/v1')
            self.assertEqual(settings[prefix+'_MODEL'], role.lower()+'-model')
            self.assertEqual(settings[prefix+'_API_KEY'], 'private-'+role)
            self.assertNotIn('private-', str(services['frontend']) + str(services['postgres']))
        self.assertEqual(settings['PROJECT_CREATOR_EMBEDDING_DIMENSIONS'], '1024')
        dependencies = services['backend']['depends_on']
        self.assertTrue(dependencies['postgres']['required'])
        self.assertFalse(dependencies['ollama']['required'])

    def test_npu_with_remote_embeddings_retains_ollama_if_local_summaries_use_it(self):
        self.app.config.ensure('npu', switch=True)
        self.configure_remote('EMBEDDING')
        self.app.config.update_env({'PROJECT_CREATOR_EMBEDDING_RUNTIME': 'openai-compatible'})
        self.app.compose('config')
        self.assertIn(str(self.root/'compose.remote.yml'), self.runner.calls[-1])
        self.assertIn(str(self.root/'compose.npu.yml'), self.runner.calls[-1])
        self.app.config.update_env({'PROJECT_CREATOR_SUMMARY_MODEL': 'local-summary', 'PROJECT_CREATOR_SUMMARY_DIGEST': 'a'*64})
        self.app.compose('config')
        self.assertNotIn(str(self.root/'compose.remote.yml'), self.runner.calls[-1])

    def test_remote_overlay_requires_compose_with_optional_dependencies(self):
        self.runner.responses = {'version --short': 'v2.20.1', 'docker info': '{"OSType":"linux"}'}
        with self.assertRaisesRegex(cli.Failure, '2.20.2'):
            self.app.prerequisites('remote')
        self.runner.responses['version --short'] = '2.20.2'
        self.app.prerequisites('remote')

    def test_remote_setup_up_and_doctor_skip_local_model_tools_and_resources(self):
        self.configure_remote('GENERATOR', 'EMBEDDING')
        self.runner.responses = {'version --short': '2.30.1', 'docker info': '{"OSType":"linux"}'}
        with patch.object(cli.Path, 'read_text', autospec=True, side_effect=Path.read_text) as read, \
             patch.object(self.app, 'inspect_gpu', side_effect=AssertionError), \
             patch.object(self.app, 'pin_model', side_effect=AssertionError), \
             patch.object(self.app, 'ollama', side_effect=AssertionError), \
             patch.object(self.app, 'verify') as verify, patch.object(self.app, 'check_ui'):
            self.app.setup('remote', switch=True)
            self.app.check_execution('GENERATOR')
            self.app.up()
            self.app.doctor()
        verify.assert_called()
        self.assertFalse(any(str(args[0]) == '/proc/meminfo' for args, _ in read.call_args_list))
        for command in self.runner.calls:
            if '-f' in command:
                self.assertIn(str(self.root/'compose.remote.yml'), command)
            self.assertFalse(any(arg in ('pull', 'model-info') for arg in command))
        state = self.app.config.path.read_text()
        self.assertNotIn('private-', state + self.app.output.getvalue() + str(self.runner.calls))

    def test_mixed_installation_with_cached_local_role_uses_prepared_disk_minimum(self):
        self.configure_remote('GENERATOR')
        self.app.config.update_env({'PROJECT_CREATOR_GENERATOR_RUNTIME': 'openai-compatible',
                                    'PROJECT_CREATOR_EMBEDDING_DIGEST': 'a'*64})
        self.runner.responses = {'version --short': 'v2.20.0', 'docker info': '{"OSType":"linux"}'}
        with patch.object(Path, 'read_text', return_value='MemTotal: 16777216 kB\nSwapTotal: 0 kB'), \
             patch.object(cli.shutil, 'disk_usage', return_value=SimpleNamespace(free=8*1024**3)):
            self.app.prerequisites('cpu')

    def test_remote_up_clears_verified_state_when_provider_configuration_is_incomplete(self):
        self.app.config.state.update(device='remote', verified=True)
        self.app.config.save()
        with self.assertRaisesRegex(cli.Failure, 'CONFIG_PROVIDER'):
            self.app.up()
        self.assertFalse(cli.Config(self.root).state['verified'])
        self.assertFalse(self.runner.calls)

    def test_remote_missing_role_config_fails_before_switch_without_echoing_values(self):
        self.configure_remote('GENERATOR')
        original = self.app.config.path.read_text()
        with patch.object(self.app, 'prerequisites'), self.assertRaisesRegex(cli.Failure, 'EMBEDDING_URL') as error:
            self.app.setup('remote', switch=True)
        self.assertEqual(self.app.config.path.read_text(), original)
        self.assertNotIn('private-', str(error.exception))
        self.assertEqual(self.runner.calls, [])

    def test_explicit_remote_role_on_cpu_only_prepares_other_role(self):
        for role, local, model in [('GENERATOR', 'EMBEDDING', 'bge-m3:567m'), ('EMBEDDING', 'GENERATOR', 'qwen2.5:7b')]:
            with self.subTest(role=role):
                self.configure_remote(role)
                self.app.config.update_env({'PROJECT_CREATOR_'+role+'_RUNTIME': 'openai-compatible'})
                with patch.object(self.app, 'prerequisites', return_value=None), patch.object(self.app, 'up'), patch.object(self.app, 'pin_model') as pin:
                    self.app.setup('cpu')
                pin.assert_called_once_with(model, local)
                self.app.config.update_env({'PROJECT_CREATOR_'+role+'_RUNTIME': '', 'PROJECT_CREATOR_'+role+'_MODEL': ''})

    def test_remote_embedding_verification_queries_synthetic_project_on_reused_indexes(self):
        self.configure_remote('EMBEDDING')
        self.app.config.update_env({'PROJECT_CREATOR_EMBEDDING_RUNTIME': 'openai-compatible'})
        self.app.config.state['fixtures'] = {'fixture': {'project': 'p1'}}
        requirements = [dict(id='a', text='one', version=1), dict(id='b', text='two', version=1)]
        def graphql(query, variables):
            if 'requirements(' in query:
                return {'requirements': requirements}
            if 'indexRequirement' in query:
                return {'indexRequirement': {'status': 'ready', 'version': 1}}
            self.assertIn('searchRequirements', query)
            self.assertEqual(variables['p'], 'p1')
            self.assertIn(variables['text'], ['one', 'two'])
            return {'searchRequirements': [{'requirementId': 'a', 'version': 1}]}
        with patch.object(self.app, 'graphql', side_effect=graphql) as calls, patch.object(self.app, 'ollama', side_effect=AssertionError):
            self.app.fixture('fixture', ['one', 'two'])
        self.assertEqual(calls.call_count, 4)


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


class AsyncAnalysisTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.app = cli.Installer(Path(self.temp.name), output=io.StringIO())
        self.app.config.ensure('cpu')
        self.expected = [{'id': 'a', 'version': 1, 'text': 'one'},
                         {'id': 'b', 'version': 1, 'text': 'two'}]
        self.completed = {'id': 'run-a', 'status': 'completed', 'outcome': 'possible_collision',
                          'stale': False, 'errorCode': '', 'sources': self.expected,
                          'findings': [{'leftId': 'a', 'rightId': 'b', 'leftVersion': 1,
                                        'rightVersion': 1, 'verdict': 'collision', 'explanation': 'Explanation.'}]}
        self.running = dict(self.completed, status='running', findings=[], outcome='')
        self.calls = []
        self.executions = []
        self.app.fixture = lambda *_: ('p', self.expected)
        self.app.check_execution = self.executions.append

    def verify_sequence(self, results, *, clock=None):
        results = iter(results)
        def graphql(query, variables):
            self.calls.append(query)
            self.assertEqual(variables, {'p': 'p'})
            field = 'startProjectAnalysis' if query.startswith('mutation') else 'projectAnalysis'
            self.assertIn(field+'(', query)
            return {field: next(results)}
        self.app.graphql = graphql
        with patch.object(cli, 'CASES', [('collision', ['one', 'two'])]), \
             patch.object(cli.time, 'sleep') as sleep, \
             patch.object(cli.time, 'monotonic', side_effect=clock or (lambda: 0)):
            self.app.verify()
        return sleep

    def test_running_analysis_is_polled_to_completion_without_starting_another_run(self):
        self.verify_sequence([self.running, self.running, self.completed])
        self.assertEqual(len(self.calls), 3)
        self.assertEqual(sum(query.startswith('mutation') for query in self.calls), 1)
        self.assertTrue(self.app.config.state['verified'])
        self.assertEqual(self.executions, ['GENERATOR'])

    def test_async_failure_keeps_verification_failed_without_restarting(self):
        with self.assertRaisesRegex(cli.Failure, 'MODEL_RESPONSE.*GENERATION_FAILED'):
            self.verify_sequence([self.running, dict(self.running, status='failed', errorCode='GENERATION_FAILED')])
        self.assertEqual(len(self.calls), 2)
        self.assertFalse(self.app.config.state['verified'])
        self.assertEqual(self.executions, [])

    def test_different_run_cannot_satisfy_wait_for_the_started_analysis(self):
        with self.assertRaisesRegex(cli.Failure, 'MODEL_RESPONSE'):
            self.verify_sequence([self.running, dict(self.completed, id='different-run')])
        self.assertEqual(len(self.calls), 2)
        self.assertFalse(self.app.config.state['verified'])

    def test_running_analysis_requires_an_id_before_polling(self):
        with self.assertRaisesRegex(cli.Failure, 'MODEL_RESPONSE'):
            self.verify_sequence([dict(self.running, id=None)])
        self.assertEqual(len(self.calls), 1)

    def test_changed_source_snapshot_is_rejected_even_while_still_running(self):
        with self.assertRaisesRegex(cli.Failure, 'MODEL_SOURCES'):
            self.verify_sequence([self.running, dict(self.running, sources=[dict(self.expected[0], version=2), self.expected[1]])])
        self.assertEqual(len(self.calls), 2)

    def test_async_wait_has_a_monotonic_deadline_and_never_restarts(self):
        with self.assertRaisesRegex(cli.Failure, 'MODEL_RESPONSE.*[Tt]im'):
            self.verify_sequence([self.running], clock=[0, 2701])
        self.assertEqual(len(self.calls), 1)
        self.assertFalse(self.app.config.state['verified'])


class UtilityTests(unittest.TestCase):
    def test_all_failure_categories_have_localized_explanations(self):
        import ast
        module = ast.parse(Path(cli.__file__).read_text())
        codes = {node.args[0].value for node in ast.walk(module)
                 if isinstance(node, ast.Call) and isinstance(node.func, ast.Name)
                 and node.func.id == 'Failure' and node.args and isinstance(node.args[0], ast.Constant)}
        codes |= {keyword.value.value for node in ast.walk(module) if isinstance(node, ast.Call)
                  for keyword in node.keywords if keyword.arg == 'code' and isinstance(keyword.value, ast.Constant)}
        for code in codes:
            self.assertIn('error.'+code, cli.MESSAGES['ru'])
            self.assertIn('error.'+code, cli.MESSAGES['en'])

    def test_russian_failure_has_localized_action_and_labeled_raw_diagnostic(self):
        result = cli.describe_failure(cli.Failure('DOCKER', 'Required executable docker is missing.'), 'ru')
        self.assertIn('Установите Compose v2.20+', result)
        self.assertIn(cli.MESSAGES['ru']['technical'], result)
        self.assertIn('Required executable docker is missing.', result)

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
