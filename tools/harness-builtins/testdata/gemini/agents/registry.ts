// Excerpt of https://github.com/google-gemini/gemini-cli/blob/v0.64.0/packages/core/src/agents/registry.ts (lines 15-23, 283-317, 336-361)
} from './types.js';
import { getAgentCardLoadOptions, getRemoteAgentTargetUrl } from './types.js';
import { loadAgentsFromDirectory } from './agentLoader.js';
import { CodebaseInvestigatorAgent } from './codebase-investigator.js';
import { CliHelpAgent } from './cli-help-agent.js';
import { GeneralistAgent } from './generalist-agent.js';
import { BrowserAgentDefinition } from './browser/browserAgentDefinition.js';
import { AgentTool } from './agent-tool.js';
import { A2AAuthProviderFactory } from './auth-provider/factory.js';
// ...
  }

  private loadBuiltInAgents(): void {
    this.registerLocalAgent(CodebaseInvestigatorAgent(this.config));
    this.registerLocalAgent(CliHelpAgent(this.config));
    this.registerLocalAgent(GeneralistAgent(this.config));

    // Register the browser agent if enabled in settings.
    // Tools are configured dynamically at invocation time via browserAgentFactory.
    const browserConfig = this.config.getBrowserAgentConfig();
    if (browserConfig.enabled) {
      // In container sandboxes (Docker/Podman/gVisor/LXC), Chrome is not
      // available inside the container. The browser agent can only work with
      // sessionMode "existing" (connecting to a host Chrome instance).
      const sandboxType = process.env['SANDBOX'];
      const isContainerSandbox =
        !!sandboxType &&
        sandboxType !== 'sandbox-exec' &&
        sandboxType !== 'sandbox:none';
      const sessionMode =
        browserConfig.customConfig.sessionMode ?? 'persistent';

      if (isContainerSandbox && sessionMode !== 'existing') {
        coreEvents.emitFeedback(
          'info',
          'Browser agent disabled in container sandbox. ' +
            'To use it, set sessionMode to "existing" in settings and start Chrome ' +
            'with --remote-debugging-port=9222 on the host.',
        );
      } else {
        this.registerLocalAgent(BrowserAgentDefinition(this.config));
      }
    }
  }

// ...
   * it will be overwritten, respecting the precedence established by the
   * initialization order.
   */
  protected async registerAgent<TOutput extends z.ZodTypeAny>(
    definition: AgentDefinition<TOutput>,
    errors?: string[],
  ): Promise<void> {
    const existing = this.agents.get(definition.name);
    if (existing && existing !== definition) {
      coreEvents.emitFeedback(
        'warning',
        `Duplicate agent name '${definition.name}' detected. ` +
          `The later definition will be ignored. ` +
          `Rename one of the agents to avoid this conflict.`,
      );
      return;
    }

    if (definition.kind === 'local') {
      this.registerLocalAgent(definition);
    } else if (definition.kind === 'remote') {
      await this.registerRemoteAgent(definition, errors);
    }
  }

  /**
