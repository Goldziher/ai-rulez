# APM fixture: JanDeDobbeleer/oh-my-posh

A real-world APM consumer manifest, vendored so the importer is exercised against
the format APM actually writes rather than a synthetic sample.

- Source: <https://github.com/JanDeDobbeleer/oh-my-posh>
- Commit: `85386c0af7b442a7e54007870c1ef40bd04215a8`
- License: MIT (see `LICENSE`, copied from the repository's `COPYING`)

Vendored from that commit:

- `in/apm.yml` — the project manifest. It declares its tools with the plural
  `targets:` list (`claude`, `copilot`) and names ten dependencies that use
  virtual paths into other repositories (`owner/repo/instructions/x.md`,
  `owner/repo/skills/name`, `owner/repo/path#ref`).
- `in/apm.lock.yaml` — the lock APM writes (`lockfile_version: "1"`). Its entries
  carry `repo_url`, `host`, `virtual_path`, `is_virtual`, `package_type`,
  `deployed_files` and `deployed_file_hashes`. The repo formatter normalised its
  quoting and indentation; the field values are unchanged.

oh-my-posh is a consumer: it holds no `.apm/` primitives of its own, so the import
is a plan of remotes only and writes nothing until `--fetch`. The fixture
therefore covers `detect`, the plural `targets:` list, the virtual-path
dependencies and the commit the lock pins them to; see
`TestAPMPlan_RealOhMyPosh` and `TestAPMConvert_RealOhMyPoshExplainsFetch`.
