---
name: owasp-quick-reference
description: OWASP Top 10 quick reference: the ten most critical web application security risks (access control, crypto, injection, insecure design, misconfiguration, vulnerable components, auth, integrity, logging, SSRF) each with a one-line mitigation. Load when reviewing code or designs for security vulnerabilities or threat modeling.
---

1. **Broken Access Control** — enforce authorization checks on every request, deny by default.
2. **Cryptographic Failures** — use strong standard algorithms, never roll your own crypto.
3. **Injection** — parameterize all queries, sanitize and validate all inputs.
4. **Insecure Design** — threat model early, validate business logic at every layer.
5. **Security Misconfiguration** — harden defaults, disable unnecessary features and endpoints.
6. **Vulnerable Components** — keep dependencies updated, audit regularly with language-specific tools.
7. **Authentication Failures** — require MFA, enforce strong passwords, implement rate limiting.
8. **Data Integrity Failures** — verify software updates, use signed artifacts and checksums.
9. **Logging Failures** — log all security events with context, protect log data from tampering.
10. **SSRF** — validate and allowlist URLs, restrict outbound network requests.
