"""Deterministic process/transport tests: no OpenVINO packages or weights needed."""
import contextlib
import errno
import http.client
from http.server import HTTPServer
import io
import json
from pathlib import Path
import socket
import tempfile
import threading
from types import SimpleNamespace
import unittest
from unittest import mock

import npu_runtime as npu
from test_runtime import request, finding


class FakeProcess:
    def __init__(self):
        self.alive = True
        self.terminated = False

    def is_alive(self):
        return self.alive

    def terminate(self):
        self.terminated = True
        self.alive = False

    def kill(self):
        self.alive = False

    def join(self, timeout=None):
        pass


class FakePipe:
    def __init__(self, responses=()):
        self.responses = list(responses)
        self.sent = []
        self.closed = False

    def send(self, value):
        self.sent.append(value)

    def poll(self, timeout=0):
        return bool(self.responses)

    def recv(self):
        if not self.responses:
            raise EOFError('fixture finished')
        return self.responses.pop(0)

    def close(self):
        self.closed = True


def runtime_fixture(responses=()):
    runtime = npu.Runtime.__new__(npu.Runtime)
    runtime.model = Path('/unused-test-model')
    runtime.profile = 'a'*64
    runtime.snapshot = {'same': (1, 2, 3, 4, 5)}
    runtime.lock = threading.Lock()
    runtime.process = FakeProcess()
    runtime.connection = FakePipe(responses)
    return runtime


class RuntimeLifecycleTests(unittest.TestCase):
    def sockets(self):
        try:
            server, client = socket.socketpair()
        except PermissionError:
            self.skipTest('host sandbox blocks socketpair')
        self.addCleanup(server.close)
        self.addCleanup(client.close)
        return server, client

    def test_timeout_kills_native_worker_and_requires_explicit_restart(self):
        server, _ = self.sockets()
        runtime = runtime_fixture()
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot):
            with self.assertRaisesRegex(ValueError, 'restart runtime'):
                runtime.compare(request(), server, timeout=-1)
            with self.assertRaisesRegex(ValueError, 'explicit restart'):
                runtime.health()
        self.assertTrue(runtime.process.terminated)
        self.assertTrue(runtime.connection.closed)
        self.assertFalse(runtime.lock.locked())

    def test_disconnected_client_cancels_native_inference(self):
        server, client = self.sockets()
        client.close()
        runtime = runtime_fixture()
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot):
            with self.assertRaisesRegex(ValueError, 'cancelled'):
                runtime.compare(request(), server)
        self.assertTrue(runtime.process.terminated)
        self.assertFalse(runtime.lock.locked())

    def test_busy_and_bad_profile_never_dispatch_to_worker(self):
        runtime = runtime_fixture()
        runtime.lock.acquire()
        with self.assertRaisesRegex(ValueError, 'busy'):
            runtime.compare(request(), None)
        runtime.lock.release()
        bad = dict(request(), profile='b'*64)
        with self.assertRaisesRegex(ValueError, 'profile'):
            runtime.compare(bad, None)
        self.assertEqual(runtime.connection.sent, [])

    def test_model_drift_rejected_before_dispatch_and_after_generation(self):
        runtime = runtime_fixture([{'finding': finding(), 'prompt_tokens': 10, 'seconds': .01}])
        with mock.patch.object(npu, 'inventory', return_value={}):
            with self.assertRaisesRegex(ValueError, 'model changed'):
                runtime.compare(request(), None)
        self.assertEqual(runtime.connection.sent, [])
        with mock.patch.object(npu, 'inventory', side_effect=[runtime.snapshot, {}]):
            with self.assertRaisesRegex(ValueError, 'model changed'):
                runtime.compare(request(), None)
        self.assertEqual(len(runtime.connection.sent), 1)
        self.assertFalse(runtime.lock.locked())

    def test_startup_rejects_concurrent_model_change_during_checksum(self):
        with mock.patch.object(npu, 'inventory', side_effect=[{'old': 1}, {'new': 2}]), \
             mock.patch.object(npu, 'verify_model'), \
             mock.patch.object(npu.multiprocessing, 'get_context') as spawn:
            with self.assertRaisesRegex(ValueError, 'changed during hash'):
                npu.Runtime(Path('/model'), {}, Path('/cache'))
            spawn.assert_not_called()

    def test_successful_result_retains_profile_and_exact_source_versions(self):
        runtime = runtime_fixture([{'finding': finding(), 'prompt_tokens': 10, 'seconds': .01}])
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
             contextlib.redirect_stdout(io.StringIO()):
            result = runtime.compare(request(), None)
        self.assertEqual(result, {'profile': runtime.profile, 'device': 'NPU', 'finding': finding()})
        self.assertFalse(runtime.lock.locked())


class WorkerTests(unittest.TestCase):
    def run_worker(self, finish):
        encoded = SimpleNamespace(input_ids=SimpleNamespace(shape=(1, 10)))
        tokenizer = SimpleNamespace(
            apply_chat_template=mock.Mock(return_value='full formatted template'),
            encode=mock.Mock(return_value=encoded),
            decode=mock.Mock(return_value=json.dumps(finding())))
        output = SimpleNamespace(tokens=[[11, 12, 13]], finish_reasons=[finish])
        pipeline = SimpleNamespace(get_tokenizer=lambda: tokenizer, generate=mock.Mock(return_value=output))
        core = SimpleNamespace(available_devices=['CPU', 'NPU'], get_property=lambda *_: 'Intel NPU fixture')
        ov = SimpleNamespace(Core=lambda: core)
        genai = SimpleNamespace(LLMPipeline=mock.Mock(return_value=pipeline),
                               GenerationFinishReason=SimpleNamespace(STOP='stop'))
        connection = FakePipe([request()])
        with mock.patch.dict('sys.modules', {'openvino': ov, 'openvino_genai': genai}), \
             mock.patch.object(npu.importlib.metadata, 'version', side_effect=lambda name: npu.VERSIONS[name]), \
             mock.patch.dict(npu.os.environ), contextlib.redirect_stderr(io.StringIO()):
            npu.worker(connection, Path('/fixture'), {'max_prompt_tokens': 2048, 'max_new_tokens': 512}, Path('/cache'))
        return connection, pipeline, tokenizer, encoded, genai

    def test_worker_passes_exact_counted_encoded_input_and_uses_npu_only(self):
        connection, pipeline, tokenizer, encoded, genai = self.run_worker('stop')
        self.assertTrue(connection.sent[0]['ready'])
        self.assertEqual(genai.LLMPipeline.call_args.args[1], 'NPU')
        self.assertIs(pipeline.generate.call_args.args[0], encoded)
        self.assertEqual(pipeline.generate.call_args.kwargs,
                         {'max_new_tokens': 512, 'do_sample': False, 'echo': False})
        tokenizer.encode.assert_called_once_with('full formatted template', add_special_tokens=False, truncation=False)
        tokenizer.decode.assert_called_once_with([11, 12, 13])
        self.assertEqual(connection.sent[1]['finding'], finding())
        self.assertTrue(connection.closed)

    def test_worker_rejects_native_length_finish_even_if_decoded_json_is_valid(self):
        connection, _, tokenizer, _, _ = self.run_worker('length')
        self.assertEqual(connection.sent[1], {'error': 'OUTPUT_LIMIT_OR_FINISH'})
        tokenizer.decode.assert_not_called()


class HTTPTests(unittest.TestCase):
    def server(self, unix=False):
        class FixtureRuntime:
            profile = 'a'*64

            def health(self):
                return {'profile': self.profile, 'device': 'NPU'}

            def compare(self, value, _client):
                npu.validate_request(value, self.profile)
                return dict(self.health(), finding=finding())

        if unix:
            directory = tempfile.TemporaryDirectory(prefix='pc-npu-')
            self.addCleanup(directory.cleanup)
            path = str(Path(directory.name)/'runtime.sock')
            try:
                server = npu.UnixHTTPServer(path, npu.Handler)
            except OSError as error:
                if error.errno in (errno.EPERM, errno.EACCES):
                    self.skipTest('host sandbox blocks AF_UNIX bind; HTTP handler covered over loopback')
                raise
            class UnixConnection(http.client.HTTPConnection):
                def connect(self):
                    self.sock = socket.socket(socket.AF_UNIX)
                    self.sock.connect(path)
            connection = UnixConnection('npu.local', timeout=2)
        else:
            server = HTTPServer(('127.0.0.1', 0), npu.Handler)
            connection = http.client.HTTPConnection(*server.server_address, timeout=2)
        server.runtime = FixtureRuntime()
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        def close():
            connection.close()
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)
        self.addCleanup(close)
        return connection

    def test_unix_socket_health_and_comparison_contract(self):
        connection = self.server(unix=True)
        connection.request('GET', '/health')
        response = connection.getresponse()
        self.assertEqual(response.status, 200)
        self.assertEqual(json.loads(response.read()), {'profile': 'a'*64, 'device': 'NPU'})
        connection.request('POST', '/compare', body=json.dumps(request()), headers={'Content-Type': 'application/json'})
        response = connection.getresponse()
        self.assertEqual(response.status, 200)
        self.assertEqual(json.loads(response.read())['finding'], finding())

    def test_handler_rejects_bad_profile_duplicate_keys_and_oversized_body(self):
        connection = self.server()
        cases = [json.dumps(dict(request(), profile='b'*64)),
                 json.dumps(request())[:-1]+',"profile":"b"}']
        for body in cases:
            connection.request('POST', '/compare', body=body)
            response = connection.getresponse()
            self.assertEqual(response.status, 422)
            self.assertIn('error', json.loads(response.read()))
        connection.request('POST', '/compare', body='{}', headers={'Content-Length': str(npu.MAX_BODY+1)})
        response = connection.getresponse()
        self.assertEqual(response.status, 422)
        response.read()


if __name__ == '__main__':
    unittest.main()
