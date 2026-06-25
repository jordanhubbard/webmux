import { agentService } from './agentService';
import { persistence } from './persistenceManager';

export interface AgentAccessResult {
  allowed: boolean;
  status: number;
  error: string;
}

export class AgentAccessError extends Error {
  status: number;

  constructor(result: AgentAccessResult) {
    super(result.error);
    this.name = 'AgentAccessError';
    this.status = result.status;
  }
}

export function getAgentAccess(agentId?: string): AgentAccessResult {
  try {
    const agentConfig = agentService.getRuntimeConfig();

    if (!agentConfig.enabled || agentConfig.definitions.length === 0) {
      return { allowed: false, status: 404, error: 'Agent sessions are not enabled' };
    }

    if (agentId && !agentConfig.definitions.some(definition => definition.id === agentId)) {
      return { allowed: false, status: 404, error: 'Agent definition not found' };
    }

    if (!agentConfig.disable_in_multi_user_mode) {
      return { allowed: true, status: 200, error: '' };
    }

    try {
      const authConfig = persistence.loadAuth();
      const userCount = authConfig.auth.users?.length ?? 0;
      if (authConfig.auth.mode === 'none' || userCount <= 1) {
        return { allowed: true, status: 200, error: '' };
      }
    } catch (err) {
      console.error('Agent access check failed:', err);
    }

    return { allowed: false, status: 403, error: 'Agent sessions are disabled in multi-user mode' };
  } catch (err) {
    return { allowed: false, status: 500, error: (err as Error).message };
  }
}
