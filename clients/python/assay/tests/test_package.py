import unittest

import assay


class PackageTest(unittest.TestCase):
    def test_exposes_bootstrap_version(self) -> None:
        self.assertEqual(assay.__version__, "0.1.0")


if __name__ == "__main__":
    unittest.main()
