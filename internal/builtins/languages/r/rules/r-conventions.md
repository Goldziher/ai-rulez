---
priority: high
---

- R 4.1+, tidyverse style guide, `styler` formatting, `lintr` linting.
- Base R pipe `|>` over magrittr `%>%`, prefer base R when tidyverse not needed.
- Package dev: `devtools`/`usethis`/`roxygen2`, `testthat` (80%+ via `covr`).
- Documentation: roxygen2 for inline docs and NAMESPACE management. `@examples` on all exports.
- Vignettes with `knitr`/`rmarkdown` for long-form documentation and tutorials.
- Error handling: `tryCatch()`/`withCallingHandlers()`, `cli::cli_abort()` for user-facing errors.
- Input validation: `checkmate` or `rlang::arg_match()` for function parameter validation.
- Security: `oysteR` for vulnerability scanning of dependencies.
- C/C++: `Rcpp` or `.Call()`, PROTECT/UNPROTECT all R objects. Rust: `extendr`/`rextendr`.
- CRAN: `R CMD check --as-cran` zero warnings/notes, maintain DESCRIPTION with proper versioning.
- Anti-patterns: `eval(parse(text=))` with user input, mixing Rcpp and raw C API, `T`/`F` instead of `TRUE`/`FALSE`.
