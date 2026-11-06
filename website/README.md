# ai-rulez docs site

The [AI-Rulez documentation](https://goldziher.github.io/ai-rulez/) site, built with
[Astro Starlight](https://starlight.astro.build) and deployed to GitHub Pages by
`.github/workflows/docs.yaml`.

## Develop

```bash
pnpm install
pnpm run dev      # local dev server at http://localhost:4321/ai-rulez/
pnpm run build    # static build into ./dist
pnpm run preview  # serve the built site
```

From the repository root, `mise run docs:dev` and `mise run docs:build` do the same.

## Layout

- `src/content/docs/**` — the documentation pages (Markdown; the home page is MDX). Each page needs a
  `title` in its front matter; link between pages with site paths such as `/ai-rulez/rules/#targets`.
- `astro.config.mjs` — site config, sidebar, and the `starlight-llms-txt` plugin (generates
  `/llms.txt`, `/llms-small.txt`, `/llms-full.txt`).
- `src/styles/custom.css` — brand theme.
- `public/` — favicon, touch icon, social card and `robots.txt`.

Some pages carry generated sections that Go tests keep in sync with the code:
`cli-reference.md` and the command index of `cli.md` (`mise run docs:cli`), the rule reference in
`strict-validation.md`, the trap table in `harness-traps.md` (`UPDATE_DOCS=1 go test ./internal/lint`),
the parity table in `mcp-server.md` (`go test ./tests/parity -update-docs`) and the table in
`user-scope.md` (`UPDATE_DOCS=1 go test ./internal/generator/userscope`).
