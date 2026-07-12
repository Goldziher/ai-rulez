---
priority: high
---

- PHP 8.2+, `declare(strict_types=1)`, typed properties, union types, enums, readonly classes.
- Formatting: PSR-12 via a fixer (e.g., PHP_CodeSniffer/phpcbf or php-cs-fixer). Static analysis: a strict analyzer (e.g., PHPStan at max level or Psalm).
- Testing: PHPUnit with `@dataProvider`, 80%+ coverage.
- Error handling: specific exceptions extending `RuntimeException`, constructor promotion for value objects.
- First-class callable syntax (`$fn = strlen(...)`) for callbacks. Arrow functions (`fn() =>`) for simple closures.
- Dependencies: Composer with `composer.lock` committed, `^` version constraints. `composer audit` in CI.
- Security: require `roave/security-advisories` as dev dependency to block vulnerable packages.
- PSR-4 autoloading exclusively — no `require`/`include` for classes.
- Intersection types for strict parameter contracts. Named arguments for readability.
- Anti-patterns: `@` suppression, `eval()`, dynamic property access, `extract()`.
