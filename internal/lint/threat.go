package lint

// FailOnThreat is the --fail-on value that gates on threat findings instead of a severity.
const FailOnThreat = "threat"

// threatCode reports the rules whose finding describes something a skill does to its user: a complete
// attack shape or an instruction to the agent. The rest stay reported, but at this gate a capability or
// a style is not a verdict: allowed-tools naming Bash, an unpinned package and a plain HTTP link are all
// common in honest skills. The list is chosen from what each rule means, and measured in
// website/src/content/docs/benchmarks.md.
func threatCode(code string) bool {
	switch code {
	case CodeSecretDetected, CodeHiddenCharacters, CodeCommentInstruction, CodeShellExec, CodeSecretInConfig,
		CodeImageExfil, CodeSelfPropagation, CodeCredentialTaint, CodeStealthCommand, CodeCrossItemChain,
		CodeCredentialedSource, CodeCredentialInstruction, CodeScriptEnvExfil, CodeAntiRefusal, CodeContextExfilProse,
		CodePersistence, CodeMetadataSSRF, CodeWebShell, CodeCryptominer, CodeReverseShell, CodeObfuscatedPhrase,
		CodeMCPMetadata, CodeCollectorHost, CodeCoercedExecution, CodePermissionWeakened, CodeDefenseDisabled:
		return true
	}
	return false
}

// IsThreat reports whether the finding fails --fail-on threat. A rule at info stays below the gate
// whatever its class. Two rules count only at error: exfil-command also warns about a secret sent in an
// authentication header, which is how an API call looks, and external-finding carries the severity
// another scanner chose.
func (f *Finding) IsThreat() bool {
	switch f.Code {
	case CodeExfilCommand, CodeExternalFinding:
		return f.Severity == SeverityError
	}
	return threatCode(f.Code) && f.Severity.AtLeast(SeverityWarning)
}
