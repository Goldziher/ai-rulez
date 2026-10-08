package commands

import (
	"io"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/samber/oops"
)

func printPublish(out io.Writer, d *publish.Dist, dir string) error {
	if publishFormat == formatJSON {
		_, err := out.Write(d.Files[publish.PlanFile])
		return oops.Wrapf(err, "write plan")
	}
	verb := "wrote"
	if publishDryRun {
		verb = "would write"
	}
	reportWriter{out}.printf("preflight   validate --strict ok | lock ok | verify --plugin ok | secrets 0\n")
	if m := d.Manifest; m.Signature != nil && publishDryRun {
		reportWriter{out}.printf("signature   would sign (%s)\n", publishSignFlagText())
	} else if m.Signature != nil {
		reportWriter{out}.printf("signature   %s (%s)\n", m.Signature.File, publishSignerText(m.Signature.Signer))
	} else if d.Manifest.Name != "" {
		reportWriter{out}.printf("signature   none\n")
	}
	if a := d.Manifest.Approval; a != nil {
		reportWriter{out}.printf("approval    %d of %d selected items approved\n", a.Approved, a.Required)
	}
	reportWriter{out}.printf("artifacts   %s %d files to %s\n", verb, len(d.Files), dir)
	for _, a := range d.Plan.Artifacts {
		reportWriter{out}.printf("            %-40s %s  %d bytes\n", a.Path, a.Digest, a.Size)
	}
	reportWriter{out}.printf("            %-40s %s  %d bytes\n", publish.PlanFile, publish.Digest(d.Files[publish.PlanFile]), len(d.Files[publish.PlanFile]))
	if d.Plan.Ref != "" {
		reportWriter{out}.printf("push        %s (manifest %s)\n", d.Plan.Ref, d.Plan.OCIDigest)
	}
	for _, c := range d.Plan.Commands {
		verb := "would run"
		if publishExecute {
			verb = "running"
		}
		reportWriter{out}.printf("%-11s %s\n", verb, shellJoin(c.Argv))
	}
	if d.Plan.NPM != nil {
		reportWriter{out}.printf("registry    %s (npm runs from an empty temporary directory with explicit --userconfig and --globalconfig; no project .npmrc applies)\n",
			publish.NPMEffectiveRegistry(*d.Plan.NPM, runner.ScrubEnv(os.Environ(), npmEnvPass, nil)))
	}
	if d.Plan.Credentials != "" {
		reportWriter{out}.printf("credentials %s\n", d.Plan.Credentials)
	}
	return nil
}

// publishSignFlagText names the signing flag of a dry run.
func publishSignFlagText() string {
	if publishSignKeyless {
		return "--sign-keyless"
	}
	return "--sign-key"
}

func publishSignerText(s publish.SignerInfo) string {
	if s.Kind == "key" {
		return "key " + s.KeyID
	}
	return s.Identity + ", issuer " + s.Issuer
}

// shellJoin renders argv so it can be pasted into a POSIX shell: arguments with
// characters outside a safe set are single-quoted. The argv itself never goes
// through a shell.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./_-") == "" {
			parts[i] = a
			continue
		}
		parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}
