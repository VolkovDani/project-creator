"""Explicit Qwen source download; uses only the existing exporter environment."""
import hashlib
import json
from pathlib import Path
import re
import sys

MODEL = 'Qwen/Qwen2.5-7B-Instruct'
PATTERNS = ['*.json', '*.safetensors', '*.txt', '*.model', '*.tiktoken', '*.jinja', 'LICENSE*', 'README*']


def sha256(path):
    digest = hashlib.sha256()
    with open(path, 'rb') as stream:
        for part in iter(lambda: stream.read(1024*1024), b''):
            digest.update(part)
    return digest.hexdigest()


def write(path, data):
    # Source progress files never contain tokens; final replace survives interruption.
    temporary = path.with_suffix(path.suffix+'.tmp')
    temporary.write_text(data)
    temporary.replace(path)


def download(root, api=None, snapshot=None):
    root = Path(root)
    root.mkdir(parents=True, exist_ok=True)
    record = root/'source.json'
    if record.exists():
        value = json.loads(record.read_text())
        if value.get('model') != MODEL or not re.fullmatch(r'[a-f0-9]{40,64}', value.get('revision', '')):
            raise ValueError('SOURCE_MANIFEST: invalid immutable source identity')
        folder = Path(value['path'])
        for name, expected in value['sha256'].items():
            if Path(name).name != name or sha256(folder/name) != expected:
                raise ValueError('SOURCE_INTEGRITY: cached snapshot changed; preserve and inspect cache before retry')
        return value
    revision_path = root/'source-revision.txt'
    revision = revision_path.read_text().strip() if revision_path.exists() else None
    if revision and not re.fullmatch(r'(?:[a-f0-9]{40}|[a-f0-9]{64})', revision):
        raise ValueError('SOURCE_REVISION: expected an immutable source commit')
    if api is None or snapshot is None:
        from huggingface_hub import HfApi, snapshot_download
        api = api or HfApi()
        snapshot = snapshot or snapshot_download
    info = api.model_info(MODEL, revision=revision, files_metadata=True)
    if not re.fullmatch(r'(?:[a-f0-9]{40}|[a-f0-9]{64})', info.sha) or (revision and revision != info.sha):
        raise ValueError('SOURCE_REVISION: repository did not return the pinned commit')
    revision = info.sha
    write(revision_path, revision+'\n')
    folder = Path(snapshot(MODEL, revision=revision, allow_patterns=PATTERNS))
    if not (folder/'LICENSE').is_file():
        raise ValueError('SOURCE_LICENSE: immutable snapshot contains no LICENSE')
    import fnmatch
    expected = {item.rfilename: item for item in info.siblings
                if '/' not in item.rfilename and any(fnmatch.fnmatch(item.rfilename, pattern) for pattern in PATTERNS)}
    if not any(name.endswith('.safetensors') for name in expected):
        raise ValueError('SOURCE_MANIFEST: snapshot contains no Safetensors weights')
    hashes = {}
    for name, item in expected.items():
        file = folder/name
        if not file.is_file():
            raise ValueError('SOURCE_INTEGRITY: incomplete source snapshot')
        actual = sha256(file)
        lfs = item.lfs
        if name.endswith('.safetensors') and (not lfs or actual != lfs.sha256):
            raise ValueError('SOURCE_INTEGRITY: weight checksum differs from immutable LFS metadata')
        if lfs and actual != lfs.sha256:
            raise ValueError('SOURCE_INTEGRITY: source checksum differs from immutable LFS metadata')
        hashes[name] = actual
    value = {'model': MODEL, 'revision': revision, 'path': str(folder.resolve()), 'license': 'Apache-2.0',
             'source': 'https://huggingface.co/'+MODEL+'/tree/'+revision, 'sha256': hashes}
    write(record, json.dumps(value, indent=2)+'\n')
    write(root/'source-path.txt', str(folder.resolve())+'\n')
    return value


if __name__ == '__main__':
    try:
        result = download(Path(sys.argv[1]))
        print('Verified source commit:', result['revision'])
    except (OSError, ValueError, KeyError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
