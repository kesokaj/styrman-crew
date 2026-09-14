# Vendored third-party skills — provenance manifest

Skills below are vendored verbatim from the open skills.sh ecosystem (SKILL.md standard).
Update by re-fetching from the pinned source and bumping the entry here; never edit
vendored content in place (fork it into a differently-named skill instead).

| Skill | Source | Pin | License | Vendored | Update via |
|-------|--------|-----|---------|----------|------------|
| grilling | github.com/mattpocock/skills (`skills/productivity/grilling`) | 6654f6b | MIT | 2026-08-31 | `npx skills add mattpocock/skills -s grilling` |
| codebase-design | github.com/mattpocock/skills (`skills/engineering/codebase-design`) | 6654f6b | MIT | 2026-08-31 | `npx skills add mattpocock/skills -s codebase-design` |
| find-skills | github.com/vercel-labs/skills (`skills/find-skills`) | 435076e | MIT | 2026-08-31 | `npx skills add vercel-labs/skills -s find-skills` |
| anti-ui-slop | uizze.com `.well-known/skills` (agentskills discovery v0.2.0), v1.2.13 | sha256:c3dc1a03…09bc08 (digest verified) | MIT | 2026-08-31 | re-fetch archive from `https://uizze.com/.well-known/skills/index.json` |

## Apache-2.0 skills — attribution recorded from their own frontmatter

These two were vendored without a row here (a third, `gcloud-auth-verification` by google, v1, Apache-2.0, was merged into `gcloud-setup` on 2026-09-13 — its three rules paraphrased under "Before asking the user to log in" — and removed). Each declares its licence and publisher
in its own `SKILL.md` frontmatter, which is the authority for attribution; the fetch
origin was not recorded at the time and is marked unknown rather than guessed.

| Skill | Publisher / author | Version | License | Source |
|-------|--------------------|---------|---------|--------|
| gke-basics | Google Cloud | 1.0.0 | Apache-2.0 | not recorded when vendored |
| terraform | Anton Babenko | 1.6.0 | Apache-2.0 | not recorded when vendored |

Apache-2.0 permits redistribution with attribution, which the table above provides.
It also asks that a copy of the licence travel with the work: this repository is not
published, and if that changes, add `LICENSE` files for these two before it does.
Re-vendoring any of them should fill in the Source column at the same time.

Notes:
- `grill-me` (a launcher for `grilling`) and `skill-repair` (repairs a `skills`-CLI manifest the
  harness does not use) were pruned 2026-09-03 after the sandbox matrix: `/grilling` does the same
  job on both CLIs, and `sh-agent adopt` replaced the manifest repair. `find-skills` (vendored verbatim)
  still names `skill-repair` in its description; harmless — the model finds no such skill — and fixed
  by the next re-fetch, not by editing the vendored file.
- `anti-ui-slop` includes its own LICENSE/NOTICE/MANIFEST from the upstream archive; the
  optional paid UIZZE MCP is NOT configured — the free playbooks work standalone.
- To add a skill: run `npx skills add <owner/repo> -s <skill> -y` from the repo root (it lands
  in `skills/` through the `.agents/skills` symlink), or copy any directory holding a `SKILL.md`
  into `skills/`, then `sh-agent install`. Add a row above. `sh-agent install` deploys exactly
  the repo's skills and removes only what it installed itself, so a skill installed by hand
  into `~/.claude/skills` survives but is neither versioned nor deployed to Antigravity —
  `sh-agent adopt <skill>` moves it in here.
