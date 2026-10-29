#!/usr/bin/env python3
"""Opt-in, offline NPU-only comparison service. Standard library except worker."""
import argparse
import hashlib
import importlib.metadata
import json
import multiprocessing
import os
from pathlib import Path
import re
import select
import socket
import socketserver
import stat
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler

VERSIONS = {'openvino': '2026.4.1', 'openvino-genai': '2026.4.1.0',
            'openvino-tokenizers': '2026.4.1.0', 'numpy': '2.5.4',
            'openvino-telemetry': '2025.2.0'}
MAX_BODY = 65536
HEX = re.compile(r'[0-9a-f]{64}')
REQUIRED_FILES = {'openvino_model.xml', 'openvino_model.bin',
                  'openvino_tokenizer.xml', 'openvino_detokenizer.xml'}


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()


def profile_id(manifest):
    return hashlib.sha256(canonical(manifest)).hexdigest()


def inventory(model, hashes=True):
    model = Path(model)
    if model.is_symlink() or not model.is_dir():
        raise ValueError('model must be a real local directory')
    result = {}
    for path in sorted(model.rglob('*')):
        if path.is_symlink():
            raise ValueError('symlinks in model directory are forbidden')
        if path.is_dir():
            continue
        info = path.stat()
        if not stat.S_ISREG(info.st_mode):
            raise ValueError('model contains a non-regular file')
        name = path.relative_to(model).as_posix()
        if hashes:
            with path.open('rb') as stream:
                result[name] = hashlib.file_digest(stream, 'sha256').hexdigest()
        else:
            result[name] = (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)
    if not REQUIRED_FILES <= result.keys():
        raise ValueError('incomplete OpenVINO model/tokenizer export')
    return result


def validate_manifest(value):
    expected = {'version', 'device', 'runtime', 'output_format', 'model_id', 'revision', 'license',
                'export_command', 'max_prompt_tokens', 'max_new_tokens', 'files'}
    if not isinstance(value, dict) or set(value) != expected:
        raise ValueError('invalid manifest fields')
    if (type(value['version']) is not int or value['version'] != 2 or value['device'] != 'NPU'
            or value['runtime'] != VERSIONS or value['output_format'] != 'json-schema-v1'):
        raise ValueError('unsupported manifest device or runtime')
    if not isinstance(value['revision'], str) or not re.fullmatch(r'(?:[0-9a-f]{40}|[0-9a-f]{64})', value['revision']):
        raise ValueError('source revision must be an immutable commit hash')
    for key in ('model_id', 'license', 'export_command'):
        if not isinstance(value[key], str) or not value[key].strip() or len(value[key]) > 4000:
            raise ValueError('source provenance is required')
    for key, upper in [('max_prompt_tokens', 32768), ('max_new_tokens', 4096)]:
        if type(value[key]) is not int or not 1 <= value[key] <= upper:
            raise ValueError('invalid explicit token limit')
    files = value['files']
    if not isinstance(files, dict) or not REQUIRED_FILES <= files.keys():
        raise ValueError('model file hashes missing')
    for name, digest in files.items():
        if not isinstance(name, str) or Path(name).is_absolute() or '..' in Path(name).parts or not isinstance(digest, str) or not HEX.fullmatch(digest):
            raise ValueError('invalid model file hash')


def create_manifest(model, model_id, revision, license_name, export_command, prompt_tokens, new_tokens):
    value = dict(version=2, device='NPU', runtime=VERSIONS, output_format='json-schema-v1', model_id=model_id,
                 revision=revision, license=license_name, export_command=export_command,
                 max_prompt_tokens=prompt_tokens, max_new_tokens=new_tokens, files=inventory(model))
    validate_manifest(value)
    return value


def verify_model(model, manifest):
    validate_manifest(manifest)
    if inventory(model) != manifest['files']:
        raise ValueError('model files differ from pinned manifest')


def strict_json(text):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError('duplicate JSON key')
            result[key] = value
        return result
    return json.loads(text, object_pairs_hook=pairs,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError('invalid JSON number')))


def valid_source(source):
    return (isinstance(source, dict) and set(source) == {'id', 'version', 'text'}
            and isinstance(source['id'], str) and 1 <= len(source['id'].encode()) <= 200
            and '\n' not in source['id'] and '\r' not in source['id']
            and type(source['version']) is int and 0 < source['version'] <= 2147483647
            and isinstance(source['text'], str) and bool(source['text'].strip())
            and len(source['text']) <= 2000)


def validate_request(value, profile):
    if not isinstance(value, dict) or set(value) != {'profile', 'system', 'pair', 'schema'}:
        raise ValueError('invalid request')
    if value['profile'] != profile or not isinstance(value['system'], str) or not 1 <= len(value['system']) <= 8000 or not isinstance(value['schema'], dict):
        raise ValueError('profile or prompt mismatch')
    pair = value['pair']
    if not isinstance(pair, dict) or set(pair) != {'left', 'right'} or not all(valid_source(pair[k]) for k in pair) or pair['left']['id'] == pair['right']['id']:
        raise ValueError('invalid source pair')


def parse_finding(text, pair):
    result = strict_json(text)
    if not isinstance(result, dict) or set(result) != {'leftId', 'rightId', 'leftVersion', 'rightVersion', 'verdict', 'explanation'}:
        raise ValueError('output does not match comparison schema')
    for side in ('left', 'right'):
        if result[side+'Id'] != pair[side]['id'] or type(result[side+'Version']) is not int or result[side+'Version'] != pair[side]['version']:
            raise ValueError('output references unknown source or version')
    if result['verdict'] not in ('collision', 'insufficient', 'compatible') or not isinstance(result['explanation'], str) or not result['explanation'].strip() or len(result['explanation']) > 2000:
        raise ValueError('invalid comparison verdict or explanation')
    return result


def format_prompt(tokenizer, request, limit):
    system = request['system'] + '\nReturn one JSON object matching this schema, without markdown:\n' + json.dumps(request['schema'], ensure_ascii=False)
    history = [{'role': 'system', 'content': system},
               {'role': 'user', 'content': json.dumps(request['pair'], ensure_ascii=False)}]
    prompt = tokenizer.apply_chat_template(history, add_generation_prompt=True)
    encoded = tokenizer.encode(prompt, add_special_tokens=False, truncation=False)
    count = int(encoded.input_ids.shape[-1])
    if count > limit:
        raise ValueError('complete prompt exceeds explicit NPU token limit; nothing was truncated')
    return encoded, count


def worker(connection, model, manifest, cache):
    """Only this child imports OpenVINO; parent can terminate native inference."""
    try:
        os.environ['HF_HUB_OFFLINE'] = '1'
        os.environ['TRANSFORMERS_OFFLINE'] = '1'
        stage = 'RUNTIME_VERSION_OR_IMPORT'
        for package, expected in VERSIONS.items():
            if importlib.metadata.version(package) != expected:
                raise ValueError('runtime version differs from pinned profile')
        import openvino as ov
        import openvino_genai as genai
        stage = 'NPU_DEVICE_UNAVAILABLE'
        core = ov.Core()
        if 'NPU' not in core.available_devices:
            raise ValueError('NPU unavailable; automatic fallback is disabled')
        name = core.get_property('NPU', 'FULL_DEVICE_NAME')
        started = time.monotonic()
        stage = 'NPU_COMPILE_OR_MODEL'
        pipeline = genai.LLMPipeline(str(model), 'NPU', {
            'MAX_PROMPT_LEN': manifest['max_prompt_tokens'],
            'MIN_RESPONSE_LEN': manifest['max_new_tokens'], 'CACHE_DIR': str(cache)})
        tokenizer = pipeline.get_tokenizer()
        connection.send({'ready': True, 'device_name': name,
                         'load_seconds': time.monotonic()-started})
        while True:
            request = connection.recv()
            try:
                stage = 'PROMPT_LIMIT_OR_TEMPLATE'
                encoded, tokens = format_prompt(tokenizer, request, manifest['max_prompt_tokens'])
                started = time.monotonic()
                stage = 'NPU_GENERATION'
                structured = genai.StructuredOutputConfig(json_schema=json.dumps(request['schema']))
                output = pipeline.generate(encoded, max_new_tokens=manifest['max_new_tokens'],
                                           do_sample=False, echo=False, structured_output_config=structured)
                stage = 'OUTPUT_LIMIT_OR_FINISH'
                if (len(output.tokens) != 1 or len(output.finish_reasons) != 1
                        or output.finish_reasons[0] != genai.GenerationFinishReason.STOP
                        or len(output.tokens[0]) >= manifest['max_new_tokens']):
                    raise ValueError('incomplete generation')
                stage = 'INVALID_JSON_OR_SOURCES'
                finding = parse_finding(tokenizer.decode(output.tokens[0]), request['pair'])
                connection.send({'finding': finding, 'prompt_tokens': tokens,
                                 'seconds': time.monotonic()-started})
            except Exception:
                print(json.dumps({'error': stage, 'device': 'NPU'}), file=sys.stderr, flush=True)
                connection.send({'error': stage})
    except BaseException:
        print(json.dumps({'error': stage, 'device': 'NPU'}), file=sys.stderr, flush=True)
        try:
            connection.send({'error': 'NPU runtime initialization failed; check pinned packages, driver, permissions and model export'})
        except (BrokenPipeError, EOFError, OSError):
            pass
    finally:
        connection.close()


class Runtime:
    def __init__(self, model, manifest, cache, startup_timeout=600):
        self.snapshot = inventory(model, hashes=False)
        verify_model(model, manifest)
        if inventory(model, hashes=False) != self.snapshot:
            raise ValueError('model changed during hash verification')
        self.model, self.manifest = model, manifest
        self.profile = profile_id(manifest)
        self.lock = threading.Lock()
        context = multiprocessing.get_context('spawn')
        self.connection, child = context.Pipe()
        self.process = context.Process(target=worker, args=(child, model, manifest, cache), daemon=True)
        self.process.start()
        child.close()
        if not self.connection.poll(startup_timeout):
            self.close()
            raise ValueError('NPU startup timeout')
        try:
            self.metadata = self.connection.recv()
        except EOFError:
            self.close()
            raise ValueError('NPU worker exited during startup') from None
        if not self.metadata.get('ready'):
            self.close()
            raise ValueError(self.metadata.get('error', 'NPU startup failed'))
        if inventory(model, hashes=False) != self.snapshot:
            self.close()
            raise ValueError('model changed during compilation')

    def close(self):
        if self.process.is_alive():
            self.process.terminate()
        self.process.join(timeout=5)
        if self.process.is_alive():
            self.process.kill()
            self.process.join(timeout=5)
        self.connection.close()

    def health(self):
        if not self.process.is_alive() or inventory(self.model, hashes=False) != self.snapshot:
            raise ValueError('NPU worker unavailable or model changed; explicit restart required')
        return {'profile': self.profile, 'device': 'NPU'}

    def compare(self, request, client, timeout=110):
        validate_request(request, self.profile)
        if not self.lock.acquire(blocking=False):
            raise ValueError('NPU busy; retry explicitly after current request')
        try:
            self.health()
            self.connection.send(request)
            deadline = time.monotonic()+timeout
            while not self.connection.poll(0.1):
                readable, _, _ = select.select([client], [], [], 0)
                disconnected = bool(readable) and client.recv(1, socket.MSG_PEEK | socket.MSG_DONTWAIT) == b''
                if disconnected or time.monotonic() >= deadline or not self.process.is_alive():
                    self.close()
                    raise ValueError('NPU request cancelled or timed out; restart runtime before retry')
            response = self.connection.recv()
            self.health()
            if 'error' in response:
                raise ValueError(response['error'])
            finding = parse_finding(json.dumps(response['finding'], ensure_ascii=False), request['pair'])
            print(json.dumps({'device': 'NPU', 'profile': self.profile,
                              'prompt_tokens': response['prompt_tokens'],
                              'seconds': response['seconds']}, ensure_ascii=False), flush=True)
            return dict(self.health(), finding=finding)
        finally:
            self.lock.release()


class UnixHTTPServer(socketserver.ThreadingMixIn, socketserver.UnixStreamServer):
    daemon_threads = True


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass  # Never log source text or generated findings.

    def reply(self, status, value):
        data = canonical(value)
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        try:
            if self.path != '/health':
                self.reply(404, {'error': 'unknown route'})
                return
            self.reply(200, self.server.runtime.health())
        except (ValueError, OSError):
            self.reply(503, {'error': 'NPU unavailable or profile changed'})

    def do_POST(self):
        try:
            self.connection.settimeout(5)
            if self.path != '/compare' or self.headers.get('Transfer-Encoding'):
                raise ValueError('invalid request')
            length = int(self.headers.get('Content-Length', '0'))
            if not 0 < length <= MAX_BODY:
                raise ValueError('invalid request size')
            data = self.rfile.read(length)
            if len(data) != length:
                raise ValueError('incomplete request')
            request = strict_json(data)
            self.reply(200, self.server.runtime.compare(request, self.connection))
        except (ValueError, TypeError, KeyError, OSError, EOFError):
            try:
                self.reply(422, {'error': 'NPU comparison rejected; check runtime, profile, source and token limits'})
            except OSError:
                pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    prepare = sub.add_parser('manifest', help='Hash an explicitly exported local model; never downloads')
    prepare.add_argument('--model', type=Path, required=True)
    prepare.add_argument('--model-id', required=True)
    prepare.add_argument('--revision', required=True)
    prepare.add_argument('--license', required=True)
    prepare.add_argument('--export-command', required=True)
    prepare.add_argument('--max-prompt-tokens', type=int, default=2048)
    prepare.add_argument('--max-new-tokens', type=int, default=512)
    prepare.add_argument('--output', type=Path, required=True)
    serve = sub.add_parser('serve')
    serve.add_argument('--model', type=Path, required=True)
    serve.add_argument('--manifest', type=Path, required=True)
    serve.add_argument('--profile', required=True, help='Expected manifest SHA256 from explicit preparation')
    serve.add_argument('--socket', type=Path, required=True)
    serve.add_argument('--cache', type=Path, required=True)
    args = parser.parse_args()
    if args.command == 'manifest':
        model = args.model.resolve()
        if args.output.resolve().is_relative_to(model):
            raise ValueError('manifest must be outside model directory')
        manifest = create_manifest(args.model, args.model_id, args.revision, args.license,
                                   args.export_command, args.max_prompt_tokens, args.max_new_tokens)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_bytes(canonical(manifest)+b'\n')
        print(profile_id(manifest))
        return
    manifest = strict_json(args.manifest.read_bytes())
    if not HEX.fullmatch(args.profile) or profile_id(manifest) != args.profile:
        raise ValueError('manifest does not match explicitly pinned profile')
    if not args.socket.is_absolute() or len(str(args.socket).encode()) > 100:
        raise ValueError('use an absolute Unix socket path under 100 bytes')
    if args.cache.resolve().is_relative_to(args.model.resolve()):
        raise ValueError('compiled cache must be outside model directory')
    args.socket.parent.mkdir(parents=True, exist_ok=True, mode=0o750)
    if args.socket.exists():
        raise ValueError('socket path exists; stop prior runtime or remove a confirmed stale socket')
    args.cache.mkdir(parents=True, exist_ok=True)
    runtime = Runtime(args.model, manifest, args.cache)
    print(json.dumps(dict(runtime.metadata, profile=runtime.profile, device='NPU')), flush=True)
    try:
        with UnixHTTPServer(str(args.socket), Handler) as server:
            os.chmod(args.socket, 0o660)
            server.runtime = runtime
            server.serve_forever()
    finally:
        runtime.close()
        args.socket.unlink(missing_ok=True)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
