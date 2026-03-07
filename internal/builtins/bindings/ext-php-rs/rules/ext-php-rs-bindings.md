---
priority: high
---
# ext-php-rs Binding Conventions
- Use `#[php_class]` and `#[php_function]` macros for PHP-visible APIs.
- Map Rust types to PHP using `ext-php-rs` type system. Handle `Zval` conversions.
- Build PHP extensions with `cargo-php`. Support PHP 8.2+.
- Use `PhpException` for error handling across the FFI boundary.
- Test bindings from PHP using PHPUnit.
