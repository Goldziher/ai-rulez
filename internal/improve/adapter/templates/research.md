# Wiring a research optimizer (recipe, not supported)

Research tools that iteratively rewrite a skill against a benchmark can sit behind `improve run`. ai-rulez
bundles no such tool and has not verified the command-line interface of any of them, so this is a recipe,
not an adapter.

1. Write a wrapper script (start from `ai-rulez improve adapters shell`) that
   - reads the request on standard input and writes the train cases (`.train_cases`) to the tool's benchmark
     format; never the held-out cases, which improve does not send;
   - runs the tool against `<skill>/SKILL.md` in the current directory, with its own budget flag set from
     `.budget.max_cost_usd`;
   - prints `{"version":1,"summary":"...","changed":["SKILL.md"],"cost_usd":<what the tool reports>}`.
2. Run it with the narrowest settings the tool allows:

   ```sh
   ai-rulez improve run <skill> --with ./optimize.sh --max-cost 5 --isolation require \
     --env-pass TOOL_API_KEY --egress api.provider.example --dry-run
   ```

   Drop `--dry-run` once the plan looks right. `--isolation require` confines writes to the run workspace and
   denies the network unless `--egress` names a host.
3. Review `ai-rulez improve show <run-id>`, then `improve apply` or `improve pr`.

Treat the tool's output as untrusted text: improve escapes `summary` and `notes`, applies the diff policy
and gates the candidate on held-out cases the tool never saw. A tool that cannot honour `editable` paths
or leaks the train cases to a service you do not trust is not safe to run here.
