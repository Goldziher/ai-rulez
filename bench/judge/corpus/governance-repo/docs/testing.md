# Testing practices

- Python test files MUST end in `_test.py` and call `unittest.main()` under `if __name__ == "__main__":`.
- Every bug fix MUST ship with a regression test.
- Tests SHOULD build shared state in a `setUp` method rather than in module-level variables.
