// Excerpt of https://github.com/XiaomiMiMo/MiMo-Code/blob/b53297d8e6f01d3cfeecb75eac59b542869901e7/packages/cli/src/command/index.ts
export const Default = {
  INIT: "init",
  DREAM: "dream",
  GOAL: "goal",
  LOOPS: "loops",
  UNUSED: "unused",
} as const;

commands[Default.INIT] = {
  name: Default.INIT,
};
// commands[Default.DREAM] = {
if (Flag.MIMOCODE_EXPERIMENTAL_CRON) {
  commands[Default.LOOPS] = {
    name: Default.LOOPS,
  };
}
commands[Default.GOAL] = {
  name: Default.GOAL,
};
