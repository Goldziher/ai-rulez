# Common failures

- `fixture 'x' not found`: the fixture is defined in a conftest outside the test's directory.
- `ImportError` only under pytest: the package is not installed in editable mode.
- Order-dependent failure: a test mutates module-level state.
