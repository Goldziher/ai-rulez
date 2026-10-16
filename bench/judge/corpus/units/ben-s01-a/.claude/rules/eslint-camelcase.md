# ESLint camelcase

Use the `camelcase` rule with `properties: never` so object keys keep the API casing.
Ignore destructured identifiers, and disable the rule for generated files. Override the default
`allow` list in `.eslintrc` rather than adding inline `eslint-disable` comments.
