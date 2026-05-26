---
priority: high
---

- Use `jni` crate for Rust-Java/JVM interop. Functions must be `#[no_mangle] pub extern "system"`.
- Follow JNI naming: `Java_com_example_ClassName_methodName`. Match Java native method signatures exactly.
- Use `JNIEnv` for all JNI operations. Access Java types via `JObject`, `JString`, `JClass` wrappers.
- String conversion: `env.get_string(&jstring)` to read, `env.new_string("value")` to create.
- Error handling: check `env.exception_check()` after JNI calls. Throw Java exceptions via `env.throw_new()`.
- Global references (`env.new_global_ref()`) for objects accessed across threads or native calls.
- Build with `cargo build --release` to produce shared library. Load in Java with `System.loadLibrary()`.
- Test Java side with JUnit, Rust core logic with standard `#[test]`. Integration tests via JNI calls.
- Keep JNI layer thin — business logic in Rust, Java provides API surface.
- Memory: JNI local references are auto-freed on native return. Use `env.delete_local_ref()` in loops.
- Anti-patterns: holding JNI references across threads without global refs, ignoring exception checks, `unwrap()` in JNI functions.
