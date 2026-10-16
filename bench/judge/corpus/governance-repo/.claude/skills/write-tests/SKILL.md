---
name: write-tests
description: "Writes a unit test for one Python function. Use when the user asks for tests of existing code; not for integration tests."
---

# Write tests

1. Create the test file named `<module>_test.py` next to the module.
2. Use this shape:

```python
import unittest

from parcel_calc import total

ITEMS = [10, 20]


class TotalTest(unittest.TestCase):
    def test_total(self):
        self.assertEqual(total(ITEMS), 30)

    def test_total_empty(self):
        self.assertEqual(total([]), 0)


if __name__ == "__main__":
    unittest.main()
```

3. Run `python <module>_test.py` and confirm that both tests pass.
