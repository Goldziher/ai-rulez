// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import starlightLlmsTxt from "starlight-llms-txt";
import { readFile, writeFile } from "node:fs/promises";

const sidebar = [
  {
    label: "Get started",
    items: [
      { label: "Installation", slug: "installation" },
      { label: "Quick start", slug: "quick-start" },
      { label: "Examples", slug: "examples" },
      { label: "Migrating to v5", slug: "migration-v5" },
    ],
  },
  {
    label: "Author",
    items: [
      { label: "Configuration", slug: "configuration" },
      { label: "Rules", slug: "rules" },
      { label: "Skills", slug: "skills" },
      { label: "Checks", slug: "checks" },
      { label: "OKF (Open Knowledge Format)", slug: "okf" },
      { label: "Domains and profiles", slug: "domains" },
      { label: "Roles", slug: "roles" },
      { label: "Includes", slug: "includes" },
      { label: "Installed skills", slug: "installed-skills" },
      { label: "Installed plugins", slug: "installed-plugins" },
      { label: ".agents as a source", slug: "agents-source" },
      { label: "Config without a .ai-rulez folder", slug: "config-without-folder" },
      { label: "Local configuration", slug: "local-overrides" },
      { label: "Catalog", slug: "catalog" },
      { label: "Monorepos", slug: "monorepo" },
    ],
  },
  {
    label: "Generate",
    items: [
      { label: "Supported harnesses", slug: "harnesses" },
      { label: "Custom presets", slug: "profiles" },
      { label: "AGENTS.md and .agents/skills", slug: "agents-md" },
      { label: "Managing only remote skills", slug: "managing-remote-skills" },
      { label: "llms.txt", slug: "llms-txt" },
      { label: "Hooks and settings", slug: "settings" },
      { label: "Permissions", slug: "permissions" },
      { label: "Run generate in remote sessions", slug: "session-start" },
      { label: "User-level configuration", slug: "user-scope" },
      { label: "Baz", slug: "baz" },
    ],
  },
  {
    label: "Bundle and publish",
    items: [
      { label: "Authoring plugins", slug: "plugins" },
      { label: "Agent Plugins", slug: "agent-plugins" },
      { label: "Agentic Resource Discovery", slug: "ard" },
      { label: "MCP server cards", slug: "mcp-server-card" },
      { label: "Publish", slug: "publish" },
    ],
  },
  {
    label: "Validate",
    items: [
      { label: "Strict validation", slug: "strict-validation" },
      { label: "Harness traps", slug: "harness-traps" },
      { label: "Verifiers", slug: "verifiers" },
      { label: "Session exposure and areas", slug: "exposure" },
      { label: "GitHub Actions", slug: "github-actions" },
      { label: "Poly hooks", slug: "poly-hooks" },
    ],
  },
  {
    label: "Govern",
    items: [
      { label: "Trust model", slug: "trust-model" },
      { label: "Lock file", slug: "lockfile" },
      { label: "Approvals", slug: "approvals" },
      { label: "Signing", slug: "signing" },
      { label: "Organization policy", slug: "policy" },
      { label: "SBOM", slug: "sbom" },
      { label: "Forge client", slug: "forge" },
    ],
  },
  {
    label: "Operate",
    items: [
      { label: "MCP server", slug: "mcp-server" },
      { label: "Skill search", slug: "search" },
      { label: "Usage telemetry", slug: "usage-telemetry" },
      { label: "Item-load telemetry (OTLP)", slug: "telemetry" },
      { label: "Evals", slug: "evals" },
      { label: "Improve (experimental)", slug: "improve" },
      { label: "Review", slug: "review" },
      { label: "Judge", slug: "judge" },
      { label: "LLM access", slug: "llm" },
      { label: "Performance", slug: "performance" },
    ],
  },
  {
    label: "Reference",
    items: [
      { label: "Architecture", slug: "architecture" },
      { label: "Standards conformance", slug: "standards" },
      { label: "CLI guide", slug: "cli" },
      { label: "CLI command reference", slug: "cli-reference" },
      { label: "Schema", slug: "schema" },
      { label: "Embedding (Go API)", slug: "embedding" },
      { label: "Architecture decision records", slug: "adr-index" },
      {
        label: "Changelog",
        link: "https://github.com/Goldziher/ai-rulez/blob/main/CHANGELOG.md",
        attrs: { target: "_blank" },
      },
    ],
  },
];

const SITE = "https://goldziher.github.io/ai-rulez/";

// llms.txt (https://llmstxt.org/#format) is an index: a title, a summary, then H2 sections that list
// links. starlight-llms-txt writes only the summary and links to llms-small.txt and llms-full.txt, and
// ends with a "## Notes" section of plain bullets, which `ai-rulez validate` reports as AR9P3. After
// the build, the notes are removed and one section per sidebar group lists its pages with their
// descriptions.
async function pageDescription(slug) {
  for (const ext of [".md", ".mdx"]) {
    try {
      const src = await readFile(new URL(`./src/content/docs/${slug}${ext}`, import.meta.url), "utf8");
      const fm = /^---\n([\s\S]*?)\n---/u.exec(src)?.[1] ?? "";
      const m = /^description:\s*(?:"((?:[^"\\]|\\.)*)"|>-?\s*\n((?:[ \t]+.*\n?)+)|(.*))$/mu.exec(fm);
      if (!m) return "";
      const raw = m[1] !== undefined ? m[1].replace(/\\(.)/gu, "$1") : (m[2] ?? m[3] ?? "");
      return raw.replace(/\s+/gu, " ").trim();
    } catch {
      // Try the next extension.
    }
  }
  return "";
}

// Maintainer-facing pages stay on the site but out of the llms.txt index.
const INDEX_SKIP = new Set(["architecture", "adr-index"]);

async function sidebarSections() {
  const out = [];
  for (const group of sidebar) {
    const lines = [];
    for (const item of group.items) {
      if (INDEX_SKIP.has(item.slug)) continue;
      const url = item.slug !== undefined ? `${SITE}${item.slug}/` : item.link;
      const desc = item.slug !== undefined ? await pageDescription(item.slug) : "";
      lines.push(`- [${item.label}](${url})${desc ? `: ${desc}` : ""}`);
    }
    out.push(`## ${group.label}\n\n${lines.join("\n")}\n`);
  }
  return out.join("\n");
}

const llmsTxtIndex = {
  name: "llms-txt-index",
  hooks: {
    "astro:build:done": async ({ dir }) => {
      const file = new URL("llms.txt", dir);
      let text = await readFile(file, "utf8");
      text = text.replace(/\n## Notes\n[\s\S]*?(?=\n## |$)/u, "\n").replace(/\n+$/u, "\n");
      const sections = await sidebarSections();
      text = text.replace(/\n## Documentation Sets\n/u, `\n${sections}\n## Documentation Sets\n`);
      await writeFile(file, text);
    },
  },
};

export default defineConfig({
  site: "https://goldziher.github.io",
  base: "/ai-rulez",
  integrations: [
    llmsTxtIndex,
    starlight({
      title: "AI-Rulez",
      description:
        "One source for the rules, skills and settings of every AI coding assistant. Author once in " +
        ".ai-rulez/, generate native files for 52 assistants, validate, lock, sign and publish.",
      logo: {
        src: "./src/assets/logo.png",
        alt: "AI-Rulez",
      },
      favicon: "/favicon-32.png",
      customCss: ["./src/styles/custom.css"],
      social: [{ icon: "github", label: "GitHub", href: "https://github.com/Goldziher/ai-rulez" }],
      editLink: {
        baseUrl: "https://github.com/Goldziher/ai-rulez/edit/main/website/",
      },
      head: [
        {
          tag: "link",
          attrs: { rel: "apple-touch-icon", href: "/ai-rulez/apple-touch-icon.png" },
        },
        {
          tag: "meta",
          attrs: { property: "og:image", content: "https://goldziher.github.io/ai-rulez/og.png" },
        },
        {
          tag: "meta",
          attrs: { name: "twitter:card", content: "summary_large_image" },
        },
        {
          tag: "meta",
          attrs: { name: "twitter:image", content: "https://goldziher.github.io/ai-rulez/og.png" },
        },
      ],
      plugins: [
        starlightLlmsTxt({
          exclude: ["cli-reference", "strict-validation", "adr-index", "performance", "architecture"],
          promote: ["index*", "installation", "quick-start", "configuration", "rules", "skills"],
          minify: { collapseCodeBlocks: true },
          details:
            "AI-Rulez keeps the rules, skills, agents, commands, hooks, permissions and MCP servers of a " +
            "project in .ai-rulez/ and generates the native files of each AI coding assistant from it. " +
            "Run `ai-rulez --help` or `ai-rulez <command> --help` for the authoritative flag list.",
        }),
      ],
      sidebar,
    }),
  ],
});
