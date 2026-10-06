package commands

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// runSignArtifact signs a plugin bundle (--bundle), a published skill (--skill)
// or an SBOM file (--sbom) and returns the exit code. None of them needs a
// project: a publisher signs a directory it ships.
func runSignArtifact(ctx context.Context, env ambient.Env) int {
	subject, target := signSubject()
	meta := signing.ArtifactMeta{Version: Version, Now: time.Now()}
	repoDir := target
	if subject == signing.SubjectSBOM {
		repoDir = filepath.Dir(target)
	}
	meta.Repository, meta.Ref = detectRepo(ctx, repoDir, env)

	var (
		st *signing.Statement
		ts signing.TreeSubject
		fs signing.FileSubject
		er error
	)
	if subject == signing.SubjectSBOM {
		st, fs, er = signing.SBOMStatement(target, meta)
	} else {
		st, ts, er = signing.TreeStatement(subject, target, meta)
	}
	if er != nil {
		fmtError(er)
		return 1
	}
	out := artifactAttestationPath(subject, target)
	if out, er = appendTarget(out); er != nil {
		fmtError(er)
		return 1
	}
	signer, err := newSigner(ctx, env)
	if err != nil {
		fmtError(err)
		return 1
	}
	if err := exportPublicKey(signer); err != nil {
		fmtError(err)
		return 1
	}
	bundle, err := signing.SignStatement(ctx, signer, st)
	if err != nil {
		fmtError(err)
		return 1
	}
	if err := writeBundle(out, bundle); err != nil {
		fmtError(err)
		return 1
	}
	digest := ts.Digest
	if subject == signing.SubjectSBOM {
		digest = "sha256:" + fs.DigestHex
	}
	info, _ := signing.Inspect(bundle) //nolint:errcheck // display only
	logger.Success("Signed "+subject, "path", target, "signer", signerLabel(info), "subject", digest, "bundle", out)
	if signProvenance {
		return signProvenanceFor(ctx, signer, ts, meta, env, out)
	}
	return 0
}

// signSubject returns the subject kind and path the flags name; validateSignFlags
// has checked that exactly one is set.
func signSubject() (subject, target string) {
	switch {
	case signBundle != "":
		return signing.SubjectBundle, signBundle
	case signSkill != "":
		return signing.SubjectSkill, signSkill
	default:
		return signing.SubjectSBOM, signSBOM
	}
}

// artifactAttestationPath is where the attestation goes: --output, else the
// sidecar inside a bundle or skill directory, else next to the SBOM file.
func artifactAttestationPath(subject, target string) string {
	if signOutput != "" {
		return signOutput
	}
	if subject == signing.SubjectSBOM {
		return target + ".sigstore.json"
	}
	return filepath.Join(target, signing.SidecarName)
}

// signProvenanceFor writes the SLSA provenance statement of a signed bundle next
// to its attestation.
func signProvenanceFor(ctx context.Context, signer signing.Signer, ts signing.TreeSubject, meta signing.ArtifactMeta, env ambient.Env, attestation string) int {
	in := signing.ProvenanceInput{
		BuilderID: provenanceBuilder(env), Version: Version, Repository: meta.Repository, Ref: meta.Ref,
		Commit: provenanceCommit(ctx, signBundle, env), InvocationID: provenanceInvocation(env), Now: meta.Now,
	}
	st, err := signing.ProvenanceStatement(ts, in)
	if err != nil {
		fmtError(err)
		return 1
	}
	bundle, err := signing.SignStatement(ctx, signer, st)
	if err != nil {
		fmtError(err)
		return 1
	}
	out := signing.ProvenanceSidecarFor(attestation)
	if err := writeBundle(out, bundle); err != nil {
		fmtError(err)
		return 1
	}
	logger.Success("Signed SLSA provenance", "builder", in.BuilderID, "bundle", out)
	return 0
}

// provenanceBuilder is the builder id to record: --builder-id, else the GitHub
// Actions workflow reference (which names the workflow file and ref that ran),
// else ai-rulez's own. Outside a CI system the id says only that the CLI ran.
func provenanceBuilder(env ambient.Env) string {
	if signBuilderID != "" {
		return signBuilderID
	}
	if wf := ambient.Getenv(env, "GITHUB_WORKFLOW_REF"); wf != "" {
		server := ambient.Getenv(env, "GITHUB_SERVER_URL")
		if server == "" {
			server = "https://github.com"
		}
		return strings.TrimSuffix(server, "/") + "/" + wf
	}
	return signing.DefaultBuilderID
}

func provenanceCommit(ctx context.Context, dir string, env ambient.Env) string {
	if sha := ambient.Getenv(env, "GITHUB_SHA"); sha != "" {
		return sha
	}
	return gitOutput(ctx, dir, "rev-parse", "HEAD")
}

func provenanceInvocation(env ambient.Env) string {
	repo, run := ambient.Getenv(env, "GITHUB_REPOSITORY"), ambient.Getenv(env, "GITHUB_RUN_ID")
	if repo == "" || run == "" {
		return ""
	}
	server := ambient.Getenv(env, "GITHUB_SERVER_URL")
	if server == "" {
		server = "https://github.com"
	}
	return strings.TrimSuffix(server, "/") + "/" + repo + "/actions/runs/" + run
}
