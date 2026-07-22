---
name: r-conventions
description: R code conventions covering R 4.1+, tidyverse/base style, styler/lintr, roxygen2, testthat, error handling, input validation, C/C++/Rust interop, and CRAN compliance. Load when writing or reviewing R code.
---

- R 4.1+, a consistent style guide (e.g., tidyverse), a formatter (e.g., styler) and a linter (e.g., lintr).
- Base R pipe `|>` over magrittr `%>%`, prefer base R when tidyverse not needed.
- Package dev: `devtools`/`usethis`/`roxygen2`, `testthat` (80%+ via a coverage tool, e.g., covr).
- Documentation: roxygen2 for inline docs and NAMESPACE management. `@examples` on all exports.
- Vignettes with `knitr`/`rmarkdown` for long-form documentation and tutorials.
- Error handling: `tryCatch()`/`withCallingHandlers()`, `cli::cli_abort()` for user-facing errors.
- Input validation: `checkmate` or `rlang::arg_match()` for function parameter validation.
- Security: a dependency vulnerability scanner (e.g., oysteR).
- C/C++: `Rcpp` or `.Call()`, PROTECT/UNPROTECT all R objects. Rust: `extendr`/`rextendr`.
- CRAN: `R CMD check --as-cran` zero warnings/notes, maintain DESCRIPTION with proper versioning.
- Anti-patterns: `eval(parse(text=))` with user input, mixing Rcpp and raw C API, `T`/`F` instead of `TRUE`/`FALSE`.
