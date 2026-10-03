import argparse
import copy
import unittest

from stability import bounded_int, verify_scan


class StabilityEvidenceTests(unittest.TestCase):
    def test_partial_budget_evidence_required(self):
        task = {"status": "succeeded", "result": {"truncated": True,
                "reason": "entries", "files": 1000}}
        verify_scan(task, 1000)
        for key, value in (("truncated", False), ("reason", "time"), ("files", 999)):
            incorrect = copy.deepcopy(task)
            incorrect["result"][key] = value
            with self.assertRaises(RuntimeError):
                verify_scan(incorrect, 1000)
        task["status"] = "failed"
        with self.assertRaises(RuntimeError):
            verify_scan(task, 1000)

    def test_oversized_evidence_rejected(self):
        with self.assertRaises(RuntimeError):
            verify_scan({"status": "succeeded", "result": {"truncated": True,
                        "reason": "entries", "files": 1000, "extra": "x" * (8 << 20)}}, 1000)

    def test_fixture_capacity_boundaries(self):
        parse = bounded_int(2000, 1000000)
        self.assertEqual(parse("2000"), 2000)
        self.assertEqual(parse("1000000"), 1000000)
        for value in ("1999", "1000001", "-1"):
            with self.assertRaises(argparse.ArgumentTypeError):
                parse(value)
