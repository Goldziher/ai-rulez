---
type: Reference
title: Test Command
description: A test command for v3.5.0
x-ai-rulez:
  kind: command
  id: test-command
  metadata:
    priority: high
    targets:
      - claude
      - cursor
      - devin
    aliases:
      - t
      - try
    usage: /test [argument]
    shortcut: cmd+shift+t
    category: testing
    name: test
---

## Test Command

This is a test command to verify the command generation system works correctly.

### Instructions

When this command is invoked:

1. Verify the command metadata is properly loaded
2. Check that the command appears in the appropriate preset outputs
3. Validate that target filtering works correctly

### Expected Behavior

The command should be generated as:

- Claude: `.claude/skills/test/SKILL.md`
- Cursor: `.cursor/rules/cmd-test.mdc`
