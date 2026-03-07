---
priority: high
---
# PyO3 Binding Conventions
- Use `#[pyclass]` and `#[pymethods]` for Python-visible types.
- Map Rust `Result<T, E>` to Python exceptions using `PyErr`.
- Use `pyo3::types` for Python type conversions. Prefer `&str` over `String` in function signatures.
- Release the GIL with `py.allow_threads()` for CPU-intensive Rust code.
- Test bindings from Python using `pytest` and `maturin develop`.
