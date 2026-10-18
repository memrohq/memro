"""
LangChain memory integration for memro.

Usage:
    from memro import Agent
    from memro.integrations.langchain import MemroMemory
    from langchain.chains import ConversationChain
    from langchain_openai import ChatOpenAI

    agent = Agent.from_env()
    memory = MemroMemory(agent=agent)

    chain = ConversationChain(llm=ChatOpenAI(), memory=memory)
    chain.predict(input="Remember that I prefer Python")
    chain.predict(input="What language do I prefer?")  # recalls from memro
"""
from typing import Any, Dict, List, Optional

try:
    from langchain_core.memory import BaseMemory
except ImportError:
    raise ImportError(
        "langchain-core is required: pip install langchain-core"
    )

from ..agent import Agent


class MemroMemory(BaseMemory):
    """
    LangChain BaseMemory backed by the memro protocol.

    Every user input and AI response is stored as an episodic memory.
    On load, semantically relevant memories are retrieved using the
    current user input as the search query.
    """

    agent: Any  # memro.Agent — typed as Any to avoid pydantic model conflicts
    memory_key: str = "history"
    input_key: Optional[str] = None
    recall_limit: int = 5

    class Config:
        arbitrary_types_allowed = True

    @property
    def memory_variables(self) -> List[str]:
        return [self.memory_key]

    def load_memory_variables(self, inputs: Dict[str, Any]) -> Dict[str, Any]:
        """Recall relevant memories for the current input."""
        query = ""
        if self.input_key and self.input_key in inputs:
            query = inputs[self.input_key]
        elif inputs:
            query = str(next(iter(inputs.values())))

        if query:
            results = self.agent.recall(query, limit=self.recall_limit)
            memories_text = "\n".join(f"- {r.content}" for r in results)
        else:
            recent = self.agent.get_recent(limit=self.recall_limit)
            memories_text = "\n".join(f"- {m.content}" for m in recent)

        return {self.memory_key: memories_text or "(no relevant memories yet)"}

    def save_context(self, inputs: Dict[str, Any], outputs: Dict[str, str]) -> None:
        """Store the human input and AI response as episodic memories."""
        input_str = ""
        if self.input_key and self.input_key in inputs:
            input_str = inputs[self.input_key]
        elif inputs:
            input_str = str(next(iter(inputs.values())))

        output_str = outputs.get("output") or outputs.get(next(iter(outputs), ""), "")

        if input_str:
            self.agent.remember(f"User said: {input_str}", type="episodic")
        if output_str:
            self.agent.remember(f"Assistant replied: {output_str}", type="episodic")

    def clear(self) -> None:
        """No-op: memro memories are persistent by design."""
        pass
