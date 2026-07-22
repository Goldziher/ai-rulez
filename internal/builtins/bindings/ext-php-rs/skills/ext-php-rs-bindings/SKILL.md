---
name: ext-php-rs-bindings
description: ext-php-rs conventions for building PHP 8.2+ native extensions from a Rust core: php_class/php_function macros, Zval conversion, PhpException mapping, php.ini loading, and PHPUnit testing. Load when generating or reviewing ext-php-rs PHP bindings for a Rust library.
---

- Use `ext-php-rs` for PHP 8.2+ native extensions. Use `#[php_class]` and `#[php_function]` macros.
- Map Rust types to PHP via `FromZval`/`IntoZval` traits. Return `PhpResult<T>` for fallible operations.
- Map Rust errors to PHP exceptions: `PhpException::from(...)` with specific classes (`InvalidArgumentException`, `RuntimeException`).
- Use `#[php_method]` for class methods. Implement `__construct` for PHP constructors.
- Build with `cargo build --release`. Load extension via `php.ini` (`extension=your_ext.so`) or PECL distribution.
- Test from PHP using PHPUnit. Verify extension loading with `php -m | grep extension_name`.
- Use `ZendClassObject<T>` for wrapping Rust structs as PHP objects.
- PHP uses reference counting — avoid circular references in wrapped Rust structs. Implement `Drop` for cleanup.
- PHP is single-threaded per request — offload CPU-intensive work to Rust, return results synchronously.
- Keep PHP extension thin — business logic in Rust, PHP provides typed interface.
- Anti-patterns: panics in PHP functions, blocking operations without timeout, raw pointer manipulation.
