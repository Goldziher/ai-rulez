// Excerpt of https://github.com/CodebuffAI/codebuff/blob/8d0e7b59e038e89d142c799f96ff8c906566c588/cli/src/utils/constants.ts
export const AGENT_MODE_TO_ID = {
  DEFAULT: HARNESS_MODE_IDS[CLI_HARNESS].DEFAULT,
  LITE: IS_FREEBUFF ? 'base2-free' : HARNESS_MODE_IDS[CLI_HARNESS].LITE,
  MAX: 'base2-max',
  PLAN: 'base2-plan',
} as const
