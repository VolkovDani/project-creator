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

    def start(self):
        self.alive = True


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
    runtime.manifest = {}
    runtime.cache = Path('/unused-test-cache')
    runtime.startup_timeout = 600
    runtime.profile = 'a'*64
    runtime.snapshot = {'same': (1, 2, 3, 4, 5)}
    runtime.lock = threading.Lock()
    runtime.process = FakeProcess()
    runtime.connection = FakePipe(responses)
    runtime.recoverable_after_cancel = False
    runtime.start_failed = False
    return runtime


class RuntimeLifecycleTests(unittest.TestCase):
    def test_lifecycle_log_is_anchored_to_repository_and_valid_call_id_is_recorded(self):
        self.assertEqual(npu.LIFECYCLE_LOG,
                         Path(npu.__file__).resolve().parents[2] / 'logs/npu/runtime-lifecycle.jsonl')
        with tempfile.TemporaryDirectory() as directory:
            log_path = Path(directory) / 'events.jsonl'
            with mock.patch.object(npu, 'LIFECYCLE_LOG', log_path):
                npu.lifecycle_event('generation_started', call_id='a'*32)
                npu.lifecycle_event('generation_started', call_id='invalid')
            lines = [json.loads(line) for line in log_path.read_text().splitlines()]
        self.assertEqual(lines[0]['call_id'], 'a'*32)
        self.assertNotIn('call_id', lines[1])

    def test_typed_runtime_errors_have_stable_safe_codes(self):
        for code in ('NPU_UNAVAILABLE', 'NPU_RECOVERY_FAILED', 'NPU_WORKER_EXIT',
                     'TIMEOUT', 'INVALID_OUTPUT', 'PROFILE_CHANGED'):
            with self.subTest(code=code):
                error = npu.RuntimeFailure(code, 'private source text')
                self.assertEqual(error.code, code)
                self.assertEqual(str(error), npu.RUNTIME_ERROR_MESSAGES[code])
        with self.assertRaises(ValueError):
            npu.RuntimeFailure('PRIVATE_DETAIL', 'secret')

    def test_lifecycle_log_is_jsonl_and_redacts_request_content(self):
        with tempfile.TemporaryDirectory() as directory:
            log_path = Path(directory) / 'runtime-lifecycle.jsonl'
            with mock.patch.object(npu, 'LIFECYCLE_LOG', log_path):
                npu.lifecycle_event('worker_recovery_started', profile='a'*64,
                                    stage='NPU_STARTUP', detail='private prompt')
            record = json.loads(log_path.read_text().strip())
        self.assertEqual({key: record[key] for key in ('event', 'profile', 'device', 'stage')},
                         {'event': 'worker_recovery_started',
                          'profile': 'a'*64, 'device': 'NPU', 'stage': 'NPU_STARTUP'})
        self.assertIsInstance(record['time'], (int, float))

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
        process = runtime.process
        connection = runtime.connection
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot):
            with self.assertRaisesRegex(ValueError, 'restart runtime'):
                runtime.compare(request(), server, timeout=-1)
            with self.assertRaisesRegex(ValueError, 'explicit restart'):
                runtime.health()
        self.assertTrue(process.terminated)
        self.assertTrue(connection.closed)
        self.assertFalse(runtime.lock.locked())

    def test_unexpected_worker_exit_is_distinct_and_not_recoverable(self):
        runtime = runtime_fixture()
        runtime.process.alive = False
        with mock.patch.object(npu, 'lifecycle_event') as log_event:
            with self.assertRaisesRegex(ValueError, 'worker exited'):
                runtime.health()
        log_event.assert_any_call('worker_exit', profile=runtime.profile,
                                  stage='NPU_HEALTH', call_id=None)
        self.assertFalse(runtime.recoverable_after_cancel)

    def test_disconnected_client_cancels_native_inference(self):
        server, client = self.sockets()
        client.close()
        runtime = runtime_fixture()
        process = runtime.process
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
             mock.patch.object(npu, 'lifecycle_event') as log_event:
            with self.assertRaisesRegex(ValueError, 'cancelled'):
                runtime.compare(request(), server)
        log_event.assert_any_call('request_cancelled', profile=runtime.profile,
                                  stage='CLIENT_DISCONNECT', call_id=None)
        self.assertTrue(process.terminated)
        self.assertFalse(runtime.lock.locked())

    def test_next_health_recovers_worker_after_client_disconnect(self):
        server, client = self.sockets()
        client.close()
        runtime = runtime_fixture()
        process = runtime.process
        restarted_process = FakeProcess()
        ready = {'ready': True, 'device_name': 'fixture', 'load_seconds': 0.1}
        restarted_connection = FakePipe([ready])
        child = FakePipe()
        context = SimpleNamespace(
            Pipe=mock.Mock(return_value=(restarted_connection, child)),
            Process=mock.Mock(return_value=restarted_process))
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
             mock.patch.object(npu, 'verify_model'), \
             mock.patch.object(npu.multiprocessing, 'get_context', return_value=context), \
             contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaisesRegex(ValueError, 'cancelled'):
                runtime.compare(request(), server)
            self.assertTrue(runtime.recoverable_after_cancel)
            self.assertEqual(runtime.health(), {'profile': runtime.profile, 'device': 'NPU'})
            restarted_connection.responses.append({'finding': finding(), 'prompt_tokens': 10, 'seconds': .01})
            result = runtime.compare(request(), server)
        context.Process.assert_called_once_with(target=npu.worker, args=(child, runtime.model, runtime.manifest, runtime.cache), daemon=True)
        context.Pipe.assert_called_once_with()
        self.assertTrue(child.closed)
        self.assertTrue(process.terminated)
        self.assertTrue(restarted_process.is_alive())
        self.assertEqual(result['finding'], finding())

    def test_recovery_startup_failure_does_not_dispatch_request(self):
        server, _ = self.sockets()
        runtime = runtime_fixture()
        runtime.recoverable_after_cancel = True
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
             mock.patch.object(npu.Runtime, '_start_worker', side_effect=ValueError('startup failed'), create=True):
            with self.assertRaisesRegex(ValueError, 'startup failed'):
                runtime.health()
            self.assertEqual(runtime.connection.sent, [])

    def test_model_drift_blocks_recovery_after_client_disconnect(self):
        server, client = self.sockets()
        client.close()
        runtime = runtime_fixture()
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot):
            with self.assertRaisesRegex(ValueError, 'cancelled'):
                runtime.compare(request(), server)
        with mock.patch.object(npu, 'inventory', return_value={}):
            with mock.patch.object(npu, 'verify_model'), \
                 mock.patch.object(npu.multiprocessing, 'get_context') as spawn:
                with self.assertRaisesRegex(ValueError, 'model changed'):
                    runtime.health()
        spawn.assert_not_called()

    def test_worker_that_cannot_be_stopped_is_not_marked_recoverable(self):
        server, client = self.sockets()
        client.close()

        class UnstoppableProcess(FakeProcess):
            def terminate(self):
                self.terminated = True

            def kill(self):
                pass

            def is_alive(self):
                return True

        runtime = runtime_fixture()
        runtime.process = UnstoppableProcess()
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot):
            with self.assertRaisesRegex(ValueError, 'could not be stopped'):
                runtime.compare(request(), server)
        self.assertFalse(runtime.recoverable_after_cancel)
        self.assertTrue(runtime.start_failed)
        with self.assertRaises(npu.RuntimeFailure) as failure:
            runtime.compare(request(), server)
        self.assertEqual(failure.exception.code, 'NPU_RECOVERY_FAILED')
        self.assertEqual(len(runtime.connection.sent), 1)

    def test_concurrent_health_checks_restart_once(self):
        runtime = runtime_fixture()
        runtime.recoverable_after_cancel = True
        process = FakeProcess()
        connection = FakePipe()
        entered, release = threading.Event(), threading.Event()

        def restart(*, call_id=None, recovery=False):
            self.assertIsNone(call_id)
            self.assertTrue(recovery)
            entered.set()
            self.assertTrue(release.wait(1))
            runtime.process = process
            runtime.connection = connection
            runtime.recoverable_after_cancel = False

        start_worker = mock.Mock(side_effect=restart)
        results = []
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
             mock.patch.object(npu.Runtime, '_start_worker', start_worker, create=True):
            first = threading.Thread(target=lambda: results.append(runtime.health()))
            second = threading.Thread(target=lambda: results.append(runtime.health()))
            first.start()
            self.assertTrue(entered.wait(1))
            second.start()
            release.set()
            first.join(1)
            second.join(1)
        self.assertFalse(first.is_alive())
        self.assertFalse(second.is_alive())
        start_worker.assert_called_once_with(call_id=None, recovery=True)
        self.assertEqual(len(results), 2)

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

    def test_failed_recovery_is_attempted_once_and_remains_unavailable(self):
        runtime = runtime_fixture()
        runtime.recoverable_after_cancel = True
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
             mock.patch.object(npu, 'verify_model'), \
             mock.patch.object(npu.multiprocessing, 'get_context',
                               side_effect=RuntimeError('private startup detail')) as spawn:
            with self.assertRaises(npu.RuntimeFailure) as first:
                runtime.health()
            self.assertEqual(first.exception.code, 'NPU_RECOVERY_FAILED')
            with self.assertRaises(npu.RuntimeFailure) as second:
                runtime.health()
        self.assertEqual(second.exception.code, 'NPU_RECOVERY_FAILED')
        self.assertFalse(runtime.recoverable_after_cancel)
        spawn.assert_called_once_with('spawn')

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
        call_id = 'c'*32
        with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
             mock.patch.object(npu, 'lifecycle_event') as log_event, \
             contextlib.redirect_stdout(io.StringIO()):
            result = runtime.compare(request(), None, call_id=call_id)
        self.assertEqual(result, {'profile': runtime.profile, 'device': 'NPU', 'finding': finding()})
        log_event.assert_any_call('generation_started', profile=runtime.profile,
                                  stage='NPU_GENERATION', call_id=call_id)
        self.assertFalse(runtime.lock.locked())


class WorkerTests(unittest.TestCase):
    def run_worker(self, finish, *, schema_error=None, generation_error=None, text=None,
                   requests=None, output=None, stderr=None):
        encoded = SimpleNamespace(input_ids=SimpleNamespace(shape=(1, 10)))
        tokenizer = SimpleNamespace(
            apply_chat_template=mock.Mock(return_value='full formatted template'),
            encode=mock.Mock(return_value=encoded),
            decode=mock.Mock(return_value=json.dumps(finding()) if text is None else text))
        if output is None:
            output = SimpleNamespace(tokens=[[11, 12, 13]], finish_reasons=[finish])
        pipeline = SimpleNamespace(get_tokenizer=lambda: tokenizer,
                                   generate=mock.Mock(return_value=output, side_effect=generation_error))
        core = SimpleNamespace(available_devices=['CPU', 'NPU'], get_property=lambda *_: 'Intel NPU fixture')
        ov = SimpleNamespace(Core=lambda: core)
        genai = SimpleNamespace(LLMPipeline=mock.Mock(return_value=pipeline),
                               StructuredOutputConfig=mock.Mock(side_effect=schema_error),
                               GenerationFinishReason=SimpleNamespace(STOP='stop', LENGTH='length',
                                                                      NONE='none', TOOL_CALL='tool_call'))
        connection = FakePipe([request()] if requests is None else requests)
        with mock.patch.dict('sys.modules', {'openvino': ov, 'openvino_genai': genai}), \
             mock.patch.object(npu.importlib.metadata, 'version', side_effect=lambda name: npu.VERSIONS[name]), \
             mock.patch.dict(npu.os.environ), contextlib.redirect_stderr(stderr or io.StringIO()):
            npu.worker(connection, Path('/fixture'), {'max_prompt_tokens': 2048, 'max_new_tokens': 512}, Path('/cache'))
        return connection, pipeline, tokenizer, encoded, genai

    def test_worker_passes_exact_counted_encoded_input_and_uses_npu_only(self):
        connection, pipeline, tokenizer, encoded, genai = self.run_worker('stop')
        self.assertTrue(connection.sent[0]['ready'])
        self.assertEqual(genai.LLMPipeline.call_args.args[1], 'NPU')
        self.assertIs(pipeline.generate.call_args.args[0], encoded)
        self.assertEqual(pipeline.generate.call_args.kwargs,
                         {'max_new_tokens': 512, 'do_sample': False, 'echo': False,
                          'structured_output_config': genai.StructuredOutputConfig.return_value})
        tokenizer.encode.assert_called_once_with('full formatted template', add_special_tokens=False, truncation=False)
        tokenizer.decode.assert_called_once_with([11, 12, 13])
        self.assertEqual(connection.sent[1]['finding'], finding())
        self.assertTrue(connection.closed)

    def test_rejected_generation_logs_only_safe_counts_and_finish_reason(self):
        cases = [
            ([[11, 12, 13]], ['length'], 'OUTPUT_NON_STOP', 3, 'LENGTH'),
            ([[11]], ['private model text'], 'OUTPUT_NON_STOP', 1, 'UNKNOWN'),
            ([[11]*512], ['stop'], 'OUTPUT_TOKEN_LIMIT', 512, 'STOP'),
            ([[11]*513], ['length'], 'OUTPUT_TOKEN_LIMIT', 513, 'LENGTH'),
            ([], ['none'], 'OUTPUT_SHAPE', None, 'NONE'),
            ([[11], [12]], ['stop', 'tool_call'], 'OUTPUT_SHAPE', None, None),
            ([[11]], [], 'OUTPUT_SHAPE', 1, None),
        ]
        for tokens, reasons, stage, count, reason in cases:
            with self.subTest(stage=stage, count=count, reason=reason):
                stderr = io.StringIO()
                output = SimpleNamespace(tokens=tokens, finish_reasons=reasons)
                connection, pipeline, tokenizer, _, _ = self.run_worker(None, output=output, stderr=stderr)
                self.assertEqual(connection.sent[1], {'error': stage})
                pipeline.generate.assert_called_once()
                tokenizer.decode.assert_not_called()
                record = {'error': stage, 'device': 'NPU', 'prompt_tokens': 10, 'max_new_tokens': 512,
                          'sequence_count': len(tokens), 'finish_reason_count': len(reasons)}
                if count is not None:
                    record['generated_tokens'] = count
                if reason is not None:
                    record['finish_reason'] = reason
                self.assertEqual(json.loads(stderr.getvalue().splitlines()[0]), record)
                runtime = runtime_fixture([connection.sent[1]])
                with mock.patch.object(npu, 'inventory', return_value=runtime.snapshot), \
                     mock.patch.object(npu, 'lifecycle_event'), self.assertRaises(npu.RuntimeFailure) as rejected:
                    runtime.compare(request(), None)
                self.assertEqual(rejected.exception.code, 'INVALID_OUTPUT')

    def test_worker_enforces_request_schema_in_native_generation(self):
        connection, pipeline, _, _, genai = self.run_worker('stop')
        genai.StructuredOutputConfig.assert_called_once_with(json_schema=json.dumps(request()['schema']))
        self.assertIs(pipeline.generate.call_args.kwargs['structured_output_config'],
                      genai.StructuredOutputConfig.return_value)
        self.assertEqual(connection.sent[1]['finding'], finding())
        pipeline.generate.assert_called_once()

    def test_worker_configures_schema_for_each_request(self):
        first, second = request(), request()
        second['schema'] = {'type': 'object', 'additionalProperties': False}
        connection, pipeline, _, _, genai = self.run_worker('stop', requests=[first, second])
        self.assertEqual(genai.StructuredOutputConfig.call_args_list, [
            mock.call(json_schema=json.dumps(value['schema'])) for value in (first, second)])
        self.assertEqual(pipeline.generate.call_count, 2)
        self.assertEqual(connection.sent[1]['finding'], finding())
        self.assertEqual(connection.sent[2]['finding'], finding())

    def test_worker_never_retries_unconstrained_when_enforcement_fails(self):
        for stage in ('schema', 'generation'):
            with self.subTest(stage=stage):
                options = {stage+'_error': ValueError('native enforcement failed')}
                connection, pipeline, tokenizer, _, _ = self.run_worker('stop', **options)
                self.assertEqual(connection.sent[1], {'error': 'NPU_GENERATION'})
                self.assertEqual(pipeline.generate.call_count, int(stage == 'generation'))
                tokenizer.decode.assert_not_called()

    def test_worker_still_rejects_duplicate_keys_without_repair_or_retry(self):
        duplicate = json.dumps(finding())[:-1]+',"explanation":"duplicate"}'
        stderr = io.StringIO()
        connection, pipeline, _, _, _ = self.run_worker('stop', text=duplicate, stderr=stderr)
        self.assertEqual(connection.sent[1], {'error': 'OUTPUT_JSON'})
        pipeline.generate.assert_called_once()
        self.assertEqual(json.loads(stderr.getvalue().splitlines()[0]),
                         {'error': 'OUTPUT_JSON', 'device': 'NPU', 'prompt_tokens': 10,
                          'max_new_tokens': 512, 'sequence_count': 1, 'finish_reason_count': 1,
                          'generated_tokens': 3, 'finish_reason': 'STOP'})


class HTTPTests(unittest.TestCase):
    def server(self, unix=False):
        class FixtureRuntime:
            profile = 'a'*64
            received_call_id = None

            def health(self, call_id=None):
                self.received_call_id = call_id
                return {'profile': self.profile, 'device': 'NPU'}

            def compare(self, value, _client, call_id=None):
                self.received_call_id = call_id
                npu.validate_request(value, self.profile)
                return {'profile': self.profile, 'device': 'NPU', 'finding': finding()}

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
        connection._npu_runtime = server.runtime
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

    def test_call_id_header_is_validated_and_propagated(self):
        connection = self.server()
        call_id = 'a'*32
        connection.request('POST', '/compare', body=json.dumps(request()),
                           headers={'X-NPU-Call-ID': call_id})
        response = connection.getresponse()
        self.assertEqual(response.status, 200)
        response.read()
        self.assertEqual(connection._npu_runtime.received_call_id, call_id)
        connection.request('GET', '/health', headers={'X-NPU-Call-ID': call_id})
        response = connection.getresponse()
        self.assertEqual(response.status, 200)
        response.read()
        self.assertEqual(connection._npu_runtime.received_call_id, call_id)
        connection.request('POST', '/compare', body=json.dumps(request()),
                           headers={'X-NPU-Call-ID': 'not-a-call-id'})
        response = connection.getresponse()
        self.assertEqual(response.status, 200)
        response.read()
        self.assertIsNone(connection._npu_runtime.received_call_id)

    def test_typed_failure_returns_only_safe_code_and_generic_message(self):
        connection = self.server()
        connection._npu_runtime.compare = mock.Mock(
            side_effect=npu.RuntimeFailure('INVALID_OUTPUT', 'private source text'))
        connection.request('POST', '/compare', body=json.dumps(request()))
        response = connection.getresponse()
        payload = json.loads(response.read())
        self.assertEqual(response.status, 502)
        self.assertEqual(payload, {'code': 'INVALID_OUTPUT', 'error': 'NPU comparison failed'})

    def test_handler_rejects_bad_profile_duplicate_keys_and_oversized_body(self):
        connection = self.server()
        cases = [json.dumps(dict(request(), profile='b'*64)),
                 json.dumps(request())[:-1]+',"profile":"b"}']
        for body in cases:
            connection.request('POST', '/compare', body=body)
            response = connection.getresponse()
            self.assertEqual(response.status, 409 if body == cases[0] else 422)
            self.assertIn('error', json.loads(response.read()))
        connection.request('POST', '/compare', body='{}', headers={'Content-Length': str(npu.MAX_BODY+1)})
        response = connection.getresponse()
        self.assertEqual(response.status, 422)
        response.read()


if __name__ == '__main__':
    unittest.main()
