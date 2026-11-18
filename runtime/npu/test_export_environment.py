"""Opt-in checks for the separately installed model export dependencies."""

import os
import unittest


@unittest.skipUnless(os.environ.get('NPU_EXPORT_ENV_SMOKE') == '1',
                     'requires the explicitly installed NPU export environment')
class ExportEnvironmentTests(unittest.TestCase):
    def test_cpu_native_ops_and_exporter_import(self):
        import torch
        from torchvision.ops import nms
        from optimum.intel import OVModelForCausalLM

        self.assertIsNone(torch.version.cuda, 'export uses the pinned CPU wheel')
        boxes = torch.tensor([[0., 0., 1., 1.], [0., 0., 1., 1.]])
        scores = torch.tensor([0.9, 0.8])
        self.assertEqual(nms(boxes, scores, 0.5).tolist(), [0])
        self.assertTrue(callable(OVModelForCausalLM.from_pretrained))


if __name__ == '__main__':
    unittest.main()
