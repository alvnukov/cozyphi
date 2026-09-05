# CozyPhi

## Agent execution

**Backend**: The selected agent runtime that owns a conversation's reasoning,
tool loop, and context management. A model provider supplies inference to a
backend; choosing a provider does not replace the backend.

**Session**: A conversation belonging to one backend and one workspace, with
an identity that can outlive the running process. Sessions are not portable
between backends.

**Turn**: One accepted user submission and the agent work it starts. Tool
calls and approval waits belong to the turn rather than starting new turns.

**Job**: Work delegated by a parent session, with its own lifecycle and a
bounded result returned to the parent. A job can use a session internally.

**Steering**: User input accepted into an active turn. Queuing another turn
and interrupting the current turn are different operations.

**Host tool**: A tool whose execution belongs to CozyPhi, including its hooks,
permission decision, and edit capabilities. A backend tool executes inside
the selected external runtime instead.

**Interaction**: A pending request for a human decision or answer during a
turn. A tool result or a parent's instruction is not a human approval.
