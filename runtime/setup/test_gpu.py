import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import cli
import gpu


class GPUTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def test_nvidia_multiple_devices_require_explicit_selection_and_pin_uuid(self):
        devices = gpu.nvidia_devices('0, GPU-aaaa, NVIDIA One, 580.1\n1, GPU-bbbb, NVIDIA Two, 580.1\n')
        with self.assertRaisesRegex(gpu.GPUError, 'GPU_SELECTION'):
            gpu.select(devices, None)
        self.assertEqual(gpu.select(devices, '1')['id'], 'GPU-bbbb')
        self.assertEqual(gpu.select(devices, 'GPU-aaaa')['name'], 'NVIDIA One')
        with self.assertRaisesRegex(gpu.GPUError, 'GPU_SELECTION'):
            gpu.select(devices, '9')

    def test_amd_rocminfo_excludes_cpu_and_rejects_unsupported_targets(self):
        data = 'Agent 1\n Name: AMD CPU\n Uuid: CPU-XX\nAgent 2\n Name: gfx1151\n Marketing Name: AMD Radeon iGPU\n Uuid: GPU-1234\n'
        devices = gpu.amd_devices(data)
        self.assertEqual(len(devices), 1)
        self.assertEqual(gpu.select(devices, None)['id'], 'GPU-1234')
        with self.assertRaisesRegex(gpu.GPUError, 'GPU_UNSUPPORTED'):
            gpu.select(gpu.amd_devices(data.replace('gfx1151', 'gfx9999')), None)

    def test_empty_or_malformed_inventory_fails_closed(self):
        for inventory in ('', 'not a GPU', '0, GPU-aaaa, Card'):
            with self.subTest(inventory=inventory), self.assertRaises(gpu.GPUError):
                gpu.select(gpu.nvidia_devices(inventory), None)

    def test_old_nvidia_driver_fails_before_model_download(self):
        with self.assertRaisesRegex(gpu.GPUError, 'DEVICE_DRIVER'):
            gpu.select(gpu.nvidia_devices('0, GPU-aaaa, NVIDIA One, 540.1'), None)

    def test_selection_survives_restart_and_requires_explicit_switch(self):
        config = cli.Config(self.root)
        config.ensure('nvidia', gpu_id='GPU-aaaa')
        project = config.state['project']
        token = config.values['PROJECT_CREATOR_TOKEN']
        config = cli.Config(self.root)
        config.ensure('nvidia')
        self.assertEqual(config.state['gpu_id'], 'GPU-aaaa')
        with self.assertRaisesRegex(cli.Failure, 'DEVICE_CHANGE'):
            config.ensure('nvidia', gpu_id='GPU-bbbb')
        config.ensure('amd', gpu_id='GPU-1234', switch=True)
        self.assertEqual(config.state['project'], project)
        self.assertEqual(config.values['PROJECT_CREATOR_TOKEN'], token)
        self.assertEqual(config.values['PROJECT_CREATOR_GPU_ID'], 'GPU-1234')

    def test_old_arc_state_keeps_existing_vulkan_selector(self):
        config = cli.Config(self.root)
        config.ensure('arc')
        config.update_env({'GGML_VK_VISIBLE_DEVICES': '1'})
        app = cli.Installer(self.root, output=io.StringIO())
        self.assertEqual(app.gpu_id('arc'), '1')
        app.config.ensure('cpu', switch=True)
        self.assertEqual(app.config.values['PROJECT_CREATOR_GPU_ID'], '')

    def test_gpu_profile_is_exclusive_and_shell_selectors_do_not_leak(self):
        for device, filename in [('nvidia', 'compose.gpu.yml'), ('amd', 'compose.amd.yml'), ('arc', 'compose.intel.yml')]:
            app = cli.Installer(self.root, output=io.StringIO())
            app.config.ensure(device, switch=True)
            calls = []
            app.runner.run = lambda args, **kw: calls.append((args, kw))
            with patch.dict('os.environ', {'CUDA_VISIBLE_DEVICES': 'evil', 'ROCR_VISIBLE_DEVICES': 'evil', 'HIP_VISIBLE_DEVICES': 'evil', 'HSA_OVERRIDE_GFX_VERSION': '9.9.9'}):
                app.compose('up', '-d')
            command, options = calls[-1]
            self.assertIn(str(self.root/filename), command)
            self.assertEqual(sum(str(x).endswith(('.gpu.yml', '.amd.yml', '.intel.yml', '.npu.yml')) for x in command), 1)
            for key in ('CUDA_VISIBLE_DEVICES', 'ROCR_VISIBLE_DEVICES', 'HIP_VISIBLE_DEVICES', 'HSA_OVERRIDE_GFX_VERSION'):
                self.assertNotIn(key, options['env'])

    def test_all_gpu_roles_require_positive_offload_and_report_it(self):
        for device in ('arc', 'nvidia', 'amd'):
            app = cli.Installer(self.root, output=io.StringIO())
            app.config.ensure(device, switch=True)
            for role in ('GENERATOR', 'EMBEDDING'):
                app.config.values['PROJECT_CREATOR_'+role+'_MODEL'] = 'model'
                app.config.values['PROJECT_CREATOR_'+role+'_DIGEST'] = 'sha256:abc'
                for amount in (0, -1, True):
                    app.ollama = lambda *_: {'models': [{'name': 'model', 'digest': 'sha256:abc', 'size_vram': amount}]}
                    with self.assertRaisesRegex(cli.Failure, 'DEVICE_EXECUTION'):
                        app.check_execution(role)
                app.ollama = lambda *_: {'models': [{'name': 'model', 'digest': 'sha256:abc', 'size_vram': 100, 'size': 200}]}
                app.check_execution(role)
                self.assertEqual(app.config.state['execution'][role]['size_vram'], 100)
                self.assertIn(role, app.output.getvalue())

    def test_doctor_different_device_never_inspects_saved_runtime_as_that_device(self):
        app = cli.Installer(self.root, output=io.StringIO())
        app.config.ensure('cpu')
        app.prerequisites = lambda *a: None
        with self.assertRaisesRegex(cli.Failure, 'DEVICE_CHANGE'):
            app.doctor('nvidia')

    def test_saved_selection_overrides_env_drift_for_every_gpu(self):
        for device, selected, key in [('nvidia', 'GPU-aaaa', 'PROJECT_CREATOR_GPU_ID'), ('amd', 'GPU-1234', 'PROJECT_CREATOR_GPU_ID'), ('arc', '1', 'GGML_VK_VISIBLE_DEVICES')]:
            app = cli.Installer(self.root, output=io.StringIO())
            app.config.ensure(device, gpu_id=selected, switch=True)
            app.config.update_env({key: 'wrong'})
            calls = []
            app.runner.run = lambda args, **kw: calls.append(kw['env'])
            app.compose('config')
            self.assertEqual(calls[-1][key], selected)

    def test_nvidia_inspection_rejects_missing_gpu_and_reports_selected_uuid(self):
        app = cli.Installer(self.root, output=io.StringIO())
        app.runner.run = lambda *a, **kw: ''
        with self.assertRaisesRegex(cli.Failure, 'DEVICE_ACCESS'):
            app.inspect_gpu('nvidia')
        app.runner.run = lambda *a, **kw: '0, GPU-aaaa, NVIDIA One, 580.1\n1, GPU-bbbb, NVIDIA Two, 580.1\n'
        with self.assertRaisesRegex(cli.Failure, 'GPU_SELECTION'):
            app.inspect_gpu('nvidia')
        self.assertEqual(app.inspect_gpu('nvidia', '1'), 'GPU-bbbb')
        self.assertIn('NVIDIA Two', app.output.getvalue())

    def test_amd_inspection_checks_kfd_and_selected_supported_target(self):
        app = cli.Installer(self.root, output=io.StringIO())
        data = 'Agent 1\n Name: gfx9999\n Uuid: GPU-9999\nAgent 2\n Name: gfx1151\n Uuid: GPU-1234\n'
        app.runner.run = lambda *a, **kw: data
        with patch('cli.os.access', return_value=False), self.assertRaisesRegex(cli.Failure, 'DEVICE_ACCESS'):
            app.inspect_gpu('amd', 'GPU-1234')
        with patch('cli.os.access', return_value=True):
            self.assertEqual(app.inspect_gpu('amd', 'GPU-1234'), 'GPU-1234')
            with self.assertRaisesRegex(cli.Failure, 'GPU_UNSUPPORTED'):
                app.inspect_gpu('amd', 'GPU-9999')

    def test_inventory_does_not_inherit_hidden_gpus_or_spoofed_targets(self):
        for device, data in [('nvidia', '0, GPU-aaaa, NVIDIA One, 580.1'), ('amd', 'Agent 1\n Name: gfx1151\n Uuid: GPU-1234\n')]:
            app = cli.Installer(self.root, output=io.StringIO())
            calls = []
            def run(args, **kw):
                calls.append(kw)
                return data
            app.runner.run = run
            with patch('cli.os.access', return_value=True), patch.dict('os.environ', {'CUDA_VISIBLE_DEVICES': '9', 'ROCR_VISIBLE_DEVICES': '9', 'HIP_VISIBLE_DEVICES': '9', 'HSA_OVERRIDE_GFX_VERSION': '11.5.1'}):
                app.inspect_gpu(device)
            for key in ('CUDA_VISIBLE_DEVICES', 'ROCR_VISIBLE_DEVICES', 'HIP_VISIBLE_DEVICES', 'HSA_OVERRIDE_GFX_VERSION'):
                self.assertNotIn(key, calls[-1]['env'])

    def test_arc_inventory_handles_mixed_vendors_and_multiple_intel_devices(self):
        sysroot = self.root/'drm'
        def vendor(node, value):
            path = sysroot/node/'device/vendor'
            path.parent.mkdir(parents=True)
            path.write_text(value)
        vendor('renderD128', '0x8086')
        vendor('renderD129', '0x1002')
        app = cli.Installer(self.root, output=io.StringIO())
        with patch('cli.Path', side_effect=lambda p: sysroot if p == '/sys/class/drm' else Path(p)):
            self.assertIsNone(app.inspect_gpu('arc'))
            with self.assertRaisesRegex(cli.Failure, 'GPU_SELECTION'):
                app.inspect_gpu('arc', 'GPU-aaaa')
            vendor('renderD130', '0x8086')
            with self.assertRaisesRegex(cli.Failure, 'GPU_SELECTION'):
                app.inspect_gpu('arc')
            self.assertEqual(app.inspect_gpu('arc', '1'), '1')

    def test_setup_pins_resolved_uuid_and_up_reuses_without_download(self):
        app = cli.Installer(self.root, output=io.StringIO())
        with patch.object(app, 'prerequisites', return_value='GPU-aaaa') as checks, patch.object(app, 'compose') as compose, patch.object(app, 'pin_model'), patch.object(app, 'check_ui'), patch.object(app, 'verify') as verify:
            app.setup('nvidia', gpu_id='0')
            self.assertEqual(cli.Config(self.root).state['gpu_id'], 'GPU-aaaa')
            checks.assert_any_call('nvidia', gpu_id='0')
            checks.assert_any_call('nvidia')
            self.assertEqual(verify.call_count, 1)
            compose.reset_mock()
            app.up()
            self.assertTrue(all('pull' not in call.args for call in compose.call_args_list))
        with patch.object(app, 'prerequisites', return_value='GPU-bbbb'), self.assertRaisesRegex(cli.Failure, 'DEVICE_CHANGE'):
            app.setup('nvidia', gpu_id='1')
        self.assertEqual(cli.Config(self.root).state['gpu_id'], 'GPU-aaaa')

    def test_doctor_reports_missing_measurement_without_inference(self):
        app = cli.Installer(self.root, output=io.StringIO())
        app.config.ensure('amd', gpu_id='GPU-1234')
        for role in ('GENERATOR', 'EMBEDDING'):
            app.config.values['PROJECT_CREATOR_'+role+'_MODEL'] = 'model'
            app.config.values['PROJECT_CREATOR_'+role+'_DIGEST'] = 'digest'
        paths = []
        def api(path):
            paths.append(path)
            return {'models': [{'name': 'model', 'digest': 'digest'}]} if path == '/api/tags' else {'models': []}
        app.ollama = api
        with patch.object(app, 'prerequisites'), patch.object(app, 'compose'), patch.object(app, 'check_ui'):
            app.doctor()
        self.assertEqual(set(paths), {'/api/tags', '/api/ps'})
        self.assertIn('GENERATOR', app.output.getvalue())
        self.assertIn('EMBEDDING', app.output.getvalue())


if __name__ == '__main__':
    unittest.main()
