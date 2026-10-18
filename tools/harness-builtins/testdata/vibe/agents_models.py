# Excerpt of https://github.com/mistralai/mistral-vibe/blob/v2.26.1/vibe/core/agents/models.py
from enum import StrEnum


class BuiltinAgentName(StrEnum):
    ASK = "ask"
    PLAN = "plan"
    ACCEPT_EDITS = "accept-edits"
    SMART_APPROVE = "smart-approve"
    AUTO_APPROVE = "auto-approve"
    EXPLORE = "explore"
    LEAN = "lean"


BUILTIN_AGENTS: dict[str, AgentProfile] = {
    BuiltinAgentName.ASK: ASK,
    BuiltinAgentName.PLAN: PLAN,
    BuiltinAgentName.ACCEPT_EDITS: ACCEPT_EDITS,
    BuiltinAgentName.SMART_APPROVE: SMART_APPROVE,
    BuiltinAgentName.AUTO_APPROVE: AUTO_APPROVE,
    BuiltinAgentName.EXPLORE: EXPLORE,
    BuiltinAgentName.LEAN: LEAN,
}
