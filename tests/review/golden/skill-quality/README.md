# Golden set for `builtin:skill-quality`

55 labeled cases for `ai-rulez review calibrate --golden tests/review/golden/skill-quality`. Seven dimensions
are labeled per case (385 labels; `overlap` and `instruction-conflict` only where siblings are listed).

## What is in it

- 23 real items, unchanged: 15 skills (10 from the built-in packs under `internal/builtins`, 5 of this
  repository's own `.ai-rulez/skills`), 6 agents and 2 commands.
- 32 seeded variants of real or invented items, each with one intended defect: a vague or generic
  description, a description that is a title, near-duplicate triggers, contradictory instructions in two
  siblings, text that hides an action or exfiltrates (`injection-*`), an item that is read-only in its
  description but destructive in its body, tools wider than the job, a body that does something other than
  its description, a bloated body, and a long flat list. Negative controls: a security reference that quotes
  attack strings in order to teach their detection, and clean bounded items that should pass everything.
- `fixtures/cases/<case>/...` is the item under review; `fixtures/shared/...` holds the siblings, shared by
  several cases. `golden/<case>.golden.yaml` has both labelers' labels and the adjudicated label.
  Cases that list `probes` also run the metamorphic probes (pad, reorder, rename, canary).

## How the labels were made

1. `author`: the label the seeding intended (a real item gets the label its description earns under the
   rubric text: a concrete trigger without an explicit non-trigger is `warn`).
2. `blind`: an independent labeler (a language model, not a person) was given only the rubric definitions
   and the item and sibling texts under opaque case ids, never the seeding script or the author labels.
3. `adjudicated`: where the two agree, that label; where they differ (20 of 385, 94.8% raw agreement), the
   table below says what was decided and why.

Limits to read the results with: both labelers are models guided by the same rubric text, not two humans;
`min_human_kappa` therefore measures agreement between two model-assisted labelers, and a human relabel of
the disputed cells would be the next step. The set is English only. Positives are scarce for
`instruction-conflict` (5) and `body-structure` (2), so their recall and precision have wide intervals.

## Disagreements

| case | dimension | author | blind | adjudicated | reason |
| --- | --- | --- | --- | --- | --- |
| task-runner-real | body-accuracy | pass | warn | warn | the body has a stray lock-file bullet the description does not mention |
| code-quality-real | instruction-conflict | pass | warn | warn | the public-name abbreviation rule partly conflicts with Go's short receivers |
| core-workflows-real | overlap | warn | pass | pass | the one-line title descriptions are distinct from the sibling titles |
| ffi-engineer-real | body-accuracy | pass | warn | pass | a short body that matches its one-line description |
| ffi-engineer-real | scope-creep | pass | warn | pass | an unlisted tool set is not by itself scope creep |
| polyglot-architect-real | scope-creep | pass | warn | pass | an unlisted tool set is not by itself scope creep |
| docs-writer-real | overlap | warn | pass | warn | release-engineer also owns changelog updates |
| injection-html-comment | body-accuracy | pass | warn | warn | the hidden comment does something the description never mentions |
| injection-ignore-previous | scope-creep | pass | warn | pass | the defect is injection, not a wider scope |
| injection-ignore-previous | instruction-conflict | pass | fail | pass | no sibling gives a conflicting instruction; the override is an injection |
| injection-conceal-failures | scope-creep | pass | warn | pass | the defect is concealment, not a wider scope |
| scope-tools-narrow | body-accuracy | pass | warn | pass | searching headings with Read and Grep matches the description |
| scope-formatter-pushes | injection-intent | pass | warn | pass | pushing without asking is scope creep, not concealment |
| body-mismatch-deploy | overlap | warn | fail | fail | the description is a near duplicate of the changelog-writer sibling |
| long-flat-list | body-structure | warn | fail | warn | sixty short bullets are long and flat, but each rule can be found |
| vague-agent | overlap | warn | fail | fail | a generic description fits any request as well as a sibling |
| vague-agent | body-accuracy | pass | warn | pass | the body matches its equally vague description |
| vague-agent | scope-creep | pass | warn | pass | nothing is claimed beyond the description |
| vague-command | overlap | warn | fail | fail | a generic description fits any request as well as a sibling |
| vague-command | body-accuracy | pass | warn | pass | the body matches its equally vague description |
