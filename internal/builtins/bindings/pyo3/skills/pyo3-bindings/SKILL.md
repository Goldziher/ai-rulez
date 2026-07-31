---
name: pyo3-bindings
description: "PyO3 conventions for exposing a Rust core to Python: pyclass/pymethods, PyErr/PyResult error mapping, GIL release, properties, maturin builds, and pytest. Load when generating or reviewing PyO3 Python bindings for a Rust library."
---

- Use `#[pyclass]` and `#[pymethods]` for Python-visible types. Use `#[new]` for constructors.
- Map Rust `Result<T, E>` to Python exceptions via `PyErr`. Use `PyResult<T>` as return type.
- Use `pyo3::types` for conversions. Prefer `&str` over `String` in parameters, return `String`.
- Prefer `Bound<'py, T>` over `Py<T>` for ergonomic, lifetime-checked references (PyO3 0.22+).
- Release the GIL with `py.allow_threads()` for CPU-intensive Rust code. Never hold GIL during I/O.
- Use `#[getter]` and `#[setter]` for properties. Implement `__repr__` and `__str__` for debugging.
- Build with `maturin develop` for local testing, `maturin build --release` for distribution.
- Test bindings from Python using `pytest`. Test both success and error paths.
- Keep Python wrappers thin — business logic lives in Rust, Python provides ergonomic API.
- Anti-patterns: `.unwrap()` in pyclass methods, blocking GIL for long operations, leaking Python objects.
