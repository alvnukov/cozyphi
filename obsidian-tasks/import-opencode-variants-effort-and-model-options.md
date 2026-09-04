---
id: import-opencode-variants-effort-and-model-options
title: Import opencode variants/effort and model options
status: done
priority: high
task_type: feature
branch: feature/import-opencode-variants-effort-and-model-options
worktree_path: .worktrees/import-opencode-variants-effort-and-model-options
created_at: "2026-09-04T14:55:34.890534Z"
updated_at: "2026-09-04T16:15:13.920927Z"
---

## Body

opencode's user config `models` entries carry `options` (reasoning_effort, chat_template_kwargs.thinking, temperature, top_p) and `variants` (low/medium/high/max, "No Thinking", `disabled: true` on some). cozyphi's import (internal/opencode, merged 2319760) currently reads only `id`/`limit` and documents options/variants as deviations.

**Decisions (user, 2026-09-05):** variants are NOT separate model entries — in opencode they surface as the effort selector (see opencode source, not assumptions); import takes EVERYTHING from options (whatever the llm layer can carry into the request; the rest documented).

**Example structure (from user's opencode.json, deepseek family):** options {reasoning_effort: high, chat_template_kwargs {thinking: true}, temperature 1.0, top_p 0.95}; variants low/high/max with own options sets, medium {disabled: true}, "No Thinking" {thinking: false}.

**Source of truth:** ~/src/opencode READ-ONLY. Extract: how effort selection maps to variants → merged options at request time (provider.ts getSDKModel region, models.dev types, session/SDK request path). Verify quotes in main session before porting.

**Cozyphi side to inventory:** llm.ModelConfig fields, protocol adapters' request-param support, /model UI effort affordance — decides how much lands request-side vs documented deviation.

**Note (2026-09-04).** 2026-09-05 diagnosis (real config ~/.config/opencode/opencode.json + auth.json, real app catalog cache ~/.cozyphi/providers.json with 178 providers): `opencode models` = 411; cozyphi import = 79. Confirmed gaps, by size:
G1 openrouter dropped (355 models): auth.json HAS openrouter key, but manager.protocolForNPM only accepts @ai-sdk/openai, @ai-sdk/openai-compatible, @ai-sdk/anthropic — @ai-sdk/openrouter (and every other npm) filtered out at catalog decode. Openrouter wire is openai-compatible chat completions → mappable to llm.ProtocolOpenAI with baseURL https://openrouter.ai/api/v1.
G2 effort/variants dropped: openai models carry variants [low/medium/high/none/xhigh] = effort levels (opencode shows ONE model, effort selector separate — user confirmed); cozyphi ModelConfig carries no effort. internal/llm ALREADY defines ReasoningEffort type.
G3 model-level options (temperature) and provider-level options beyond baseURL/apiKey (openai: reasoningEffort, reasoningSummary, store, textVerbosity, include) silently dropped — providerOptions parses only baseURL+apiKey.
G4 catalog data source divergence: opencode bundles its models.dev fork (@opencode-ai/core/models-dev, snapshot: openai=19 curated models incl. gpt-5.6, zai has glm-5.2-highspeed, no gpt-4/embeddings junk); cozyphi fetches LIVE models.dev (openai=50+ incl. embeddings/gpt-image/realtime/o1-o4). Junk models in /model picker come from live data, not from a missing filter (opencode also lists openrouter *-image models).
G5 "opencode" zen provider (7 free models, no auth entry, no config entry) — enters opencode's list somehow (bundled db? special-case?) — needs source extraction.
G6 auth.json openai entry is OAuth (access/refresh/expires/accountId, no key) → cozyphi shows its models with key=false (unusable). opencode refreshes via its own OAuth app. Separate feature; likely documented deviation for now.
G7 provider instances ("lmstudio -Ubu"/"-Win" with npm+options.baseURL) and model ids with suffixes ("deepseek-v4-pro[1m]") import OK today (space ids pass through, bracket id is just the id).
Parity confirmed OK: anthropic provider options.baseURL correctly applies to ALL its models (deepseek anthropic-compatible endpoint), enabled/disabled absent, credential ladder works for key-type auth.
Scope decision for this task: G1+G2+G3 in-scope (import + request params); G4 documented data-source deviation (bundled fork vs live, unless user wants a bundled snapshot); G5 extract then decide; G6 deviation note + keep listing.

**Note (2026-09-04).** 2026-09-05 E-table (effort/variants/options semantics, ~/src/opencode @4161695; all file:line verified this session):
E1 Config model merge (provider.ts:1468-1552): options = mergeDeep(catalogModel.options, configModel.options) [1532]. Variants: base = catalog model with same npm → its variants, else ProviderTransform.variants(parsedModel) [1543-1546]; merged = mergeDeep(base, configModel.variants) [1547]; pickBy !disabled → disabled:true REMOVES variant, disabled key stripped [1549-1550]. Second pass for auth/env providers 1676-1686. Catalog path: variants = reasoningVariants(modelsDevModel) ?? variants(base) [1289, 1240-1294].
E2 Generation (transform.ts): reasoningVariants reads models.dev reasoning_options: effort→effortVariants (string|null→"none") [1654-1684]; budget_tokens→high=mid/max=min(output-1,32k-1) [1686-1699]; toggle→alibaba enableThinking/cohere thinking [1705-1717]; [] suppresses; undefined→heuristic variants() [727+]: requires capabilities.reasoning; deepseek-chat/r1/v3, minimax, glm(!glm52), kimi, qwen → NO variants [779-791]; glm-5.2: openrouter high/xhigh, openai-compat high/max reasoningEffort, anthropic high/max [751-769]; openai efforts per gpt-5 version/pro/codex/chat, none gated by release≥2025-11-13, xhigh≥2025-12-04 [574-653]; anthropic ≥4.7 adaptive low..max [655-682]; google thinkingLevel [688-718]. Variant value = per-npm option blob: openrouter {reasoning:{effort}}, anthropic {effort}/thinking adaptive, google thinkingConfig, bedrock reasoningConfig [1719-1753].
E3 Request merge (session/llm/request.ts:80-131): variant = small?{} : model.variants[user.model.variant]; options = merge(merge(merge(base, model.options), agent.options), variant) — VARIANT WINS [91]. base = built-ins per npm (store:false openai, openrouter usage.include, zai thinking enabled, google includeThoughts…) [transform.ts:1157-1261]; smallOptions = first variant fragment [1327]. temperature only if capabilities.temperature: agent ?? per-family default (kimi 0.6/1.0, glm-4.6/4.7 1.0, minimax-m2 1.0; claude undefined) [124-126, 528-545]; topP agent ?? topP(model) (gemini/kimi-k2.5/minimax-m2 0.95, deepseek-v4-flash 0.95) [127, 547-561]; topK minimax/gemini [128, 563-572]; maxOutputTokens = min(limit.output,32k)||32k [1418]. Final providerOptions = {[sdkKey(npm)]: opts}, openai npm reasoning gate forceReasoning [1358-1416].
E4 Selection (session/prompt.ts:646-685): variant from explicit /model selector > agent.variant (only if agent model matches); stored per session, "default"=none [session.ts:92,219; prompt.ts:625].
E5 disabled:true = variant removed entirely [1549, 1683-1686].
E6 "No Thinking" = plain config model entry (own map key, options.thinking:false), NOT a variant — imports as separate model already.
E7 opencode zen provider (provider.ts:185-207): no auth/env/config key → keep only cost.input==0 models, autoload with apiKey "public".
A2 cozyphi llm today: ModelConfig.ReasoningEffort+ReasoningEfforts [] (types.go:76-84) but enum ONLY minimal/low/medium/high — none/xhigh/max missing, ParseReasoningEffort rejects them [38-43]; openai client sends reasoning_effort [openai/client.go:46-48,135]; responses client validates [responses/client.go:225-228]; manager fills ReasoningEfforts for codex-subscription + zai GLM-5.2+ only [manager.go:161-175,514-515]; NO temperature/topP/topK/chat_template_kwargs anywhere in llm layer; effort picker exists in /model (manager-side lists).
Port scope implied: G2+G3 (variants/options into ModelConfig + requests via openai/anthropic/responses adapters; enum +none/xhigh) with documented deviations for per-npm blobs cozyphi protocols cannot express. G1 openrouter protocol, G4 bundled catalog fork, G5 zen provider, G6 OAuth → separate follow-ups (recorded above).

**Started (2026-09-04).** Worktree .worktrees/import-opencode-variants-effort-and-model-options, branch feature/import-opencode-variants-effort-and-model-options. E-table in task body. Port scope: G2+G3 only.

**Done (2026-09-04).** G2+G3 done: effort ladder widened to none…max (ParseReasoningEffort case-normalizes), ModelConfig gained typed Options + Variants with variant-wins deep merge (llm.MergeOptions/EffectiveOptions) delivered by openai/responses/anthropic clients; opencode source imports model options+variants (disabled dropped, keys lowercased), effort-named variants feed the /model picker. Review fixes: thinking-heuristic fold in openai applyModelOptions (config wins), ReasoningEffortOrder unexported, ptr→new. Deviations in doc/opencode.md. Landed as 4de5722, merged to main e8afd75. Follow-ups: G1 openrouter, G4 catalog fork, G5 zen, G6 OAuth.
