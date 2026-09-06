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

## Subscription-aware routing

**Quality tier**: A user-assigned quality rank (`low`, `medium`, `high`, or
`xhigh`) for an execution profile, not a universal benchmark score.
_Avoid_: native effort, task model level.

**Execution profile**: A model paired with its actual provider-native effort
and assigned exactly one quality tier by the user.
_Avoid_: model alone.

**Native effort**: The reasoning setting actually honored for a model by its
provider, distinct from the quality requested for work.

**Required tier**: The minimum quality tier requested for work, inherited by
children unless explicitly changed. A user's scoped exception may allow less.
_Avoid_: effective effort.

**Actual tier**: The quality tier of the execution profile currently used for
inference; an opportunistic upgrade does not change the required tier.

**Subscription account**: The provider account whose allowances are shared
by all its consumers, including other local processes and external applications.
_Avoid_: provider ID, session budget.

**Quota window**: An allowance with its own applicability and reset horizon;
one inference may consume several overlapping windows.

**Protected reserve**: Allowance retained for important work, adapted to
expected demand and time until reset.

**Useful surplus**: Allowance predicted to expire unused after commitments
and protected reserve, available to improve already-assigned work.
_Avoid_: spending target.

**Consumption reservation**: A claim against expected account consumption
for admitted work, pending reconciliation with observed usage.

**Active work**: Work on the user's latest assignment, irrespective of which
session is visible; other ordinary assigned work is background work.

**Routing exception**: A user's explicit, scoped permission to depart from
an automatic routing rule after its concrete risk has been disclosed.
_Avoid_: model approval, unrestricted root access.