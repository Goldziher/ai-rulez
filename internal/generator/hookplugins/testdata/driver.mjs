// Test driver: loads a generated hook plugin with a fake host and replays calls against it.
// usage: node driver.mjs <module> <flavor> <dir> <calls-json>
// Prints a JSON array with the blocking reason (or null) of every call.
import { pathToFileURL } from "node:url";

const [, , file, flavor, dir, callsJSON] = process.argv;
process.chdir(dir);
const mod = await import(pathToFileURL(file).href);
const calls = JSON.parse(callsJSON);
const results = [];
const sleep = (ms) =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

async function attempt(fn) {
  try {
    return (await fn()) ?? null;
  } catch (error) {
    return { threw: error.message };
  }
}

const adapters = {
  async "opencode-v1"() {
    const hooks = await mod.default.server({ directory: dir });
    return {
      async tool_before(call) {
        const r = await attempt(() =>
          hooks["tool.execute.before"]({ tool: call.tool, sessionID: "s1", callID: "c1" }, { args: call.input }),
        );
        return r?.threw ?? null;
      },
      async tool_after(call) {
        await hooks["tool.execute.after"](
          { tool: call.tool, sessionID: "s1", callID: "c1", args: call.input },
          { title: "t", output: "o" },
        );
        return null;
      },
      async session_start() {
        await hooks.event({ event: { type: "session.created", properties: { info: { id: "s1" } } } });
        return null;
      },
      async stop() {
        await hooks.event({ event: { type: "session.idle", properties: { sessionID: "s1" } } });
        return null;
      },
    };
  },
  async "opencode-v2"() {
    const registered = {};
    const events = [];
    const ctx = {
      location: { directory: dir },
      tool: { hook: async (name, fn) => void (registered[name] = fn) },
      event: {
        subscribe: () => ({
          async *[Symbol.asyncIterator]() {
            while (events.length > 0) yield events.shift();
          },
        }),
      },
    };
    return {
      async setup() {
        events.push(
          { type: "session.created", properties: { sessionID: "s1" } },
          { type: "session.idle", properties: { sessionID: "s1" } },
        );
        await mod.default.setup(ctx);
        await sleep(300);
      },
      async tool_before(call) {
        const r = await attempt(() =>
          registered["execute.before"]({ tool: call.tool, sessionID: "s1", input: call.input }),
        );
        return r?.threw ?? null;
      },
      async tool_after(call) {
        await registered["execute.after"]({
          tool: call.tool,
          sessionID: "s1",
          input: call.input,
          status: "completed",
          result: {},
        });
        return null;
      },
    };
  },
  async pi() {
    const handlers = {};
    mod.default({ on: (name, fn) => void (handlers[name] = fn) });
    const ctx = {
      cwd: dir,
      hasUI: false,
      sessionManager: { getSessionId: () => "s1", getSessionFile: () => "/t.jsonl" },
    };
    return {
      async tool_before(call) {
        return (
          (await handlers.tool_call({ toolName: call.tool, toolCallId: "c1", input: call.input }, ctx))?.reason ?? null
        );
      },
      async tool_after(call) {
        await handlers.tool_result(
          { toolName: call.tool, toolCallId: "c1", input: call.input, content: [], isError: Boolean(call.error) },
          ctx,
        );
        return null;
      },
      async session_start() {
        await handlers.session_start({ reason: "startup" }, ctx);
        return null;
      },
      async stop() {
        await handlers.agent_settled({}, ctx);
        return null;
      },
      async prompt(call) {
        const r = await handlers.input({ text: call.text, source: "interactive" }, ctx);
        return r.action === "handled" ? "handled" : null;
      },
    };
  },
  async amp() {
    const handlers = {};
    mod.default({ on: (name, fn) => void (handlers[name] = fn) });
    return {
      async tool_before(call) {
        const r = await handlers["tool.call"]({
          tool: call.tool,
          toolUseID: "c1",
          input: call.input,
          thread: { id: "s1" },
        });
        return r.action === "reject-and-continue" ? r.message : null;
      },
      async tool_after(call) {
        await handlers["tool.result"]({
          tool: call.tool,
          toolUseID: "c1",
          input: call.input,
          status: call.error ? "error" : "done",
          thread: { id: "s1" },
        });
        return null;
      },
      async session_start() {
        await handlers["session.start"]({ thread: { id: "s1" } });
        return null;
      },
      async stop() {
        await handlers["agent.end"]({ thread: { id: "s1" }, status: "done", messages: [] });
        return null;
      },
      async prompt(call) {
        await handlers["agent.start"]({ thread: { id: "s1" }, message: call.text });
        return null;
      },
    };
  },
};

const host = await adapters[flavor]();
if (host.setup) await host.setup();
for (const call of calls) {
  results.push(await host[call.kind](call));
}
process.stdout.write(JSON.stringify(results));
