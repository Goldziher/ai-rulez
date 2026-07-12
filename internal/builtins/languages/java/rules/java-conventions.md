---
priority: high
---

- Java 17+ LTS, records for immutable data, sealed classes for restricted hierarchies, pattern matching.
- A consistent style guide and auto-formatter (e.g., Google Java Style + google-java-format). Static analysis (e.g., Error Prone + SpotBugs).
- Build: Maven or Gradle with wrapper scripts (`mvnw`/`gradlew`). Commit wrapper files.
- Testing: JUnit 5 with `@ParameterizedTest`, an assertion library (e.g., AssertJ), a coverage tool (e.g., JaCoCo) at 80%+.
- Benchmarking: JMH for microbenchmarks — never use `System.nanoTime()` loops.
- Error handling: specific exceptions only, never `catch (Exception)`, use try-with-resources for `AutoCloseable`.
- `var` for obvious types, `Optional<T>` for returns (never params/fields), `final` fields by default.
- Prefer constructor injection over field injection for DI. Immutable collections: `List.of()`, `Map.of()`.
- Streams for collection transforms, `instanceof` pattern matching, switch expressions.
- Security: OWASP `dependency-check` Maven/Gradle plugin for CVE scanning.
- Dependencies: commit lock files, use BOM for version alignment, avoid `SNAPSHOT` in releases.
- Anti-patterns: public fields, mutable static state, raw types, checked exceptions in lambdas.
