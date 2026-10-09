package schema

// JSONContract binds one `--format json` document of the CLI to the JSON Schema
// that describes it. Command is the cobra path of the command followed by the
// flag that selects the document when one command prints several ("lock
// --check"). The tests run every contract and validate the output; the table in
// docs/cli.md ("JSON contracts") lists the same pairs.
type JSONContract struct {
	Command string
	Schema  string
}

// JSONContracts lists the report commands of the CLI and the schema of their
// JSON output. Every document carries a top-level "schema_version"; a failure
// prints ErrorDocumentSchema instead.
var JSONContracts = []JSONContract{
	{"validate", "validate-report.schema.json"},
	{"scan", "validate-report.schema.json"},
	{"doctor", "doctor-report.schema.json"},
	{"tokens", "tokens-report.schema.json"},
	{"cost", "cost-report.schema.json"},
	{"catalog", "catalog.v1.schema.json"},
	{"catalog diff", "catalog-diff.schema.json"},
	{"verify --attestation", "verify-attestation.schema.json"},
	{"verify --approvals", "verify-approvals.schema.json"},
	{"verify --plugin", "verify-plugin.schema.json"},
	{"lock --check", "lock-diff.schema.json"},
	{"lock --diff", "lock-diff.schema.json"},
	{"lock --outdated", "lock-outdated.schema.json"},
	{"lock --subject", "lock-subject.schema.json"},
	{"update", "update.schema.json"},
	{"sbom", "sbom-report.schema.json"},
	{"sign", "sign-report.schema.json"},
	{"scanners list", "scanners-list.schema.json"},
	{"scanners doctor", "scanners-doctor.schema.json"},
	{"roles list", "roles-manifest.schema.json"},
	{"roles show", "roles-show.schema.json"},
	{"roles resolve", "roles-resolve.schema.json"},
	{"verifiers run", "verifiers-report.schema.json"},
	{"verifiers list", "verifiers-list.schema.json"},
	{"verifiers test", "verifiers-test.schema.json"},
	{"verifiers explain", "verifiers-explain.schema.json"},
	{"export okf", "export-okf.schema.json"},
	{"okf validate", "okf-validate.schema.json"},
	{"publish", "publish-plan.schema.json"},
	{"publish emit", "publish-emit.schema.json"},
	{"publish verify", "publish-verify.schema.json"},
	{"eval run", "eval-report.schema.json"},
}

// ErrorDocumentSchema describes what a command with a --format flag prints on
// stdout when it fails before it can produce its report.
const ErrorDocumentSchema = "error-document.schema.json"
