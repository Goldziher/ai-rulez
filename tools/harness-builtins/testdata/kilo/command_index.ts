// Excerpt of https://github.com/Kilo-Org/kilocode/blob/v7.8.14/packages/opencode/src/command/index.ts (lines 51-54, 108-129)
export const Default = {
  INIT: "init",
  REVIEW: "review",
} as const
// ...
commands[Default.INIT] = {
  name: Default.INIT,
  description: "guided AGENTS.md setup",
  source: "command",
  get template() {
    return PROMPT_INITIALIZE.replace("${path}", ctx.worktree)
  },
  hints: hints(PROMPT_INITIALIZE),
}
// kilocode_change start
commands[Default.REVIEW] = reviewCommand()
commands.goal = {
  name: "goal",
  description:
    "Keep working toward a session goal. /goal <objective> or pause, resume, clear. A goal may wait on a scheduled wakeup, a cron task, or a background process: the wait suspends the goal until it fires and the goal resumes itself; /goal pause and /goal clear cancel its armed timers. If the objective is to wait for a deploy, build, or CI job, schedule that wait immediately; do not explore the repository first.",
  source: "command",
  template: "$ARGUMENTS",
  hints: ["<objective | pause | resume | clear>"],
}
commands["resume-claude"] = SessionResume.resumeClaude
commands["resume-codex"] = SessionResume.resumeCodex
// kilocode_change end
