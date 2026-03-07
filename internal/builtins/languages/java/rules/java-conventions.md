---
priority: high
---
# Java Conventions
- Target Java 17+ (LTS). Use records for data classes. Use sealed classes where appropriate.
- Follow Google Java Style Guide. Use pattern matching for instanceof.
- Use JUnit 5 for testing. Prefer constructor injection for dependency injection.
- Handle exceptions specifically — avoid catching `Exception` or `Throwable` broadly.
- Use `var` for local variables when the type is obvious from context.
