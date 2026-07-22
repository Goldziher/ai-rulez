---
name: csharp-conventions
description: C# code conventions covering .NET 8+/C# 12, nullable reference types, Roslyn analyzers, xUnit, record types, async/await discipline, and dependency security. Load when writing or reviewing C# code.
---

- .NET 8+ C# 12, file-scoped namespaces, primary constructors, `<Nullable>enable</Nullable>`.
- Formatting: `dotnet format` + Roslyn analyzers, `<TreatWarningsAsErrors>true</TreatWarningsAsErrors>`.
- Style enforcement: `.editorconfig` for consistent rules across team. Use an analyzer set (e.g., StyleCop.Analyzers or Roslynator).
- Testing: xUnit with `[Theory]`/`[InlineData]`, an assertion library (e.g., FluentAssertions), a coverage tool (e.g., coverlet) at 80%+.
- Benchmarking: a benchmark harness (e.g., BenchmarkDotNet) for performance testing — never `Stopwatch` loops.
- `record` types for immutable data, `required` properties, pattern matching in switch expressions.
- Async: `async`/`await` throughout — never `.Result` or `.Wait()`, `ValueTask` for hot paths, avoid deadlocks.
- Security: `dotnet list package --vulnerable` for CVE scanning. Zero tolerance for critical/high.
- Dependencies: NuGet `PackageReference`, commit `packages.lock.json`, `Directory.Build.props` for shared config.
- Collection expressions (`[1, 2, 3]`), `required` modifier, raw string literals for multi-line.
- Anti-patterns: `dynamic`, `object` params, unguarded `catch (Exception)`, `#pragma warning disable`.
