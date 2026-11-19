"""GPU inventory parsing; no downloads, driver installation or inference."""
import csv
import re

GPU_DEVICES = ('arc', 'nvidia', 'amd', 'amd-vulkan')
VULKAN_DEVICES = ('arc', 'amd-vulkan')
DEVICES = ('cpu', *GPU_DEVICES, 'npu', 'remote')
OVERRIDES = {'arc': 'compose.intel.yml', 'nvidia': 'compose.gpu.yml',
             'amd': 'compose.amd.yml', 'amd-vulkan': 'compose.amd-vulkan.yml',
             'npu': 'compose.npu.yml'}
# Ollama v0.40.16 docs/gpu.mdx, Linux ROCm targets. Never spoof a target.
AMD_TARGETS = frozenset(('gfx908', 'gfx90a', 'gfx942', 'gfx950', 'gfx1030',
                         'gfx1100', 'gfx1101', 'gfx1102', 'gfx1150', 'gfx1151',
                         'gfx1200', 'gfx1201'))


class GPUError(ValueError):
    def __init__(self, code, detail):
        self.code = code
        super().__init__(f'{code}: {detail}')


def nvidia_devices(text):
    devices = []
    for row in csv.reader(text.splitlines(), skipinitialspace=True):
        if len(row) != 4 or not row[0].isdigit() or not re.fullmatch(r'GPU-[a-fA-F0-9-]+', row[1]):
            raise GPUError('GPU_SELECTION', 'Invalid nvidia-smi inventory; inspect nvidia-smi -L.')
        devices.append(dict(index=row[0], id=row[1], name=row[2], driver=row[3], backend='CUDA'))
    return devices


def amd_devices(text):
    devices = []
    for block in re.split(r'(?m)^\s*Agent\s+\d+\s*$', text):
        target = re.search(r'(?m)^\s*Name:\s*(gfx[0-9a-f]+)\s*$', block)
        if not target:
            continue
        uuid = re.search(r'(?m)^\s*Uuid:\s*(GPU-[a-fA-F0-9]+)\s*$', block)
        name = re.search(r'(?m)^\s*Marketing Name:\s*(.+)$', block)
        devices.append(dict(index=str(len(devices)), id=uuid[1] if uuid else '',
                            name=name[1].strip() if name else target[1],
                            target=target[1], backend='ROCm'))
    return devices


def select(devices, identifier):
    if not devices:
        raise GPUError('DEVICE_ACCESS', 'No GPU found; inspect the vendor driver and inventory utility.')
    if identifier is None and len(devices) != 1:
        raise GPUError('GPU_SELECTION', 'Multiple GPUs found. Use --gpu-id from nvidia-smi or rocminfo; see docs/setup-cli.md.')
    selected = devices[0] if identifier is None else next((d for d in devices if identifier in (d['id'], d['index'])), None)
    if not selected or not selected['id']:
        raise GPUError('GPU_SELECTION', 'Selected GPU is absent or has no stable UUID; inspect vendor inventory.')
    if selected.get('target') and selected['target'] not in AMD_TARGETS:
        raise GPUError('GPU_UNSUPPORTED', 'AMD target is outside the pinned ROCm matrix; no target override or CPU fallback enabled.')
    if selected.get('driver') and (not re.fullmatch(r'[0-9]+(?:\.[0-9]+)*', selected['driver']) or int(selected['driver'].split('.')[0]) < 550):
        raise GPUError('DEVICE_DRIVER', 'Ollama v0.40.16 needs NVIDIA driver 550+; some older GPUs require 570+. See the pinned hardware matrix.')
    return selected
