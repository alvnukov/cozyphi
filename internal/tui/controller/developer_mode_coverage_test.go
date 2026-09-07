package controller

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// The coverage inventory. For every way the harness can be told something —
// a key in a config file, a variable in the environment, a flag on the
// command line, a limit built into the code — it says one of three things:
// which catalog field reports it, why nothing reports it, or which ticket
// will make something report it.
//
// It exists because "the model can read the harness" is a promise about
// completeness, and completeness is the one property a collector's own tests
// cannot show: each of them proves its own fields are right, and none of them
// can notice a setting nobody wrote a collector for. Checked against the
// source on one side and against a live registry on the other, the inventory
// notices — a setting added to a loader and to nothing else fails a test the
// day it is added, and so does a field that leaves the catalog.
//
// The third answer is the honest one. A gap is a filed ticket, not a
// justification: calling an unreported setting "unavailable" would close the
// promise by renaming it.

// catalogRef is where a setting is reported: one category, one field key.
type catalogRef struct {
	category diag.Category
	key      string
}

// gap is a setting nothing reports yet, and the ticket that will.
type gap struct {
	ticket string
	why    string
}

// coverageTable is one surface kind's account of itself.
type coverageTable struct {
	// kind is how a setting of this table is given to the harness.
	kind surfaceKind
	// reported maps a setting to the field that carries it.
	reported map[string]catalogRef
	// withheld maps a setting to why the catalog will never carry it.
	withheld map[string]string
	// missing maps a setting to the ticket that will make it reportable.
	missing map[string]gap
}

// ids is every setting the table accounts for, however it accounts for it.
func (t coverageTable) ids() []string {
	all := slices.Collect(maps.Keys(t.reported))
	all = append(all, slices.Collect(maps.Keys(t.withheld))...)
	all = append(all, slices.Collect(maps.Keys(t.missing))...)
	slices.Sort(all)
	return all
}

// inventory is the whole table, in the order a reader meets the harness:
// what a file says, what the environment says, what the command line says,
// and what nothing says because it is built in.
func inventory() []coverageTable {
	return []coverageTable{configCoverage, envCoverage, flagCoverage, runtimeCoverage}
}

// The one ticket the inventory currently owes. It is a child of the read-only
// developer mode epic and blocks its acceptance, which is the point: a gap
// that blocks nothing is a gap nobody closes.
const ticketSettingTail = "developer-mode-settings-tail"

// skippedRoots are schema-shaped structs that are not schemas. Their fields
// are skipped rather than listed, because listing them would put a second
// copy of the model section in the inventory and teach a reader nothing. A
// new one has to be named here, so the skip stays a decision.
var skippedRoots = map[string]string{
	"cmd:configDoc":       "рендер примера config.yaml для `cozyphi config`, зеркало project:fileConfig",
	"cmd:modelDoc":        "то же зеркало для секции models",
	"cmd:permDoc":         "то же зеркало для секции permissions",
	"cmd:bashDoc":         "то же зеркало для permissions.bash",
	"cmd:agentsDoc":       "то же зеркало для секции agents",
	"internal/tasks:note": "формат заметки реестра задач: данные задачи, а не настройка harness",
}

// configCoverage accounts for every config key the loaders decode.
var configCoverage = coverageTable{
	kind:     surfaceConfig,
	reported: reportedConfig,
	withheld: withheldConfig,
	missing:  missingConfig,
}

var reportedConfig = map[string]catalogRef{
	"config.yaml:agents.context_limit":              {diag.CategoryContext, diag.KeyContextCeiling},
	"config.yaml:agents.models":                     {diag.CategoryAgents, diag.KeyAgentsModels},
	"config.yaml:compaction":                        {diag.CategoryContext, diag.KeyContextCompactThreshold},
	"config.yaml:notifications":                     {diag.CategoryUI, diag.KeyNotifyMode},
	"config.yaml:opencode":                          {diag.CategoryModel, diag.KeyModelImport},
	"config.yaml:opencode.enabled":                  {diag.CategoryModel, diag.KeyModelImport},
	"config.yaml:permissions.dangerously_allow_all": {diag.CategoryPermissions, diag.KeyPermissionBypass},
	"config.yaml:permissions.tasks":                 {diag.CategoryPermissions, diag.KeyPermissionTasks},
	"config.yaml:plan.defaults":                     {diag.CategoryPlan, diag.KeyPlanPolicyTypes},
	"config.yaml:voice.stt.model":                   {diag.CategoryUI, diag.KeyVoiceModel},

	"internal/harnesssettings:Compaction.reminder_tokens":    {diag.CategoryContext, diag.KeyContextCompactThreshold},
	"internal/harnesssettings:notificationsFileConfig.mode":  {diag.CategoryUI, diag.KeyNotifyMode},
	"internal/harnesssettings:notificationsFileConfig.sound": {diag.CategoryUI, diag.KeyNotifySound},
	"internal/harnesssettings:openCodeConfig.enabled":        {diag.CategoryModel, diag.KeyModelImport},

	"internal/plangate:Defaults.types":                 {diag.CategoryPlan, diag.KeyPlanPolicyTypes},
	"internal/plangate:Defaults.additional_exemptions": {diag.CategoryPlan, diag.KeyPlanPolicyExemptions},
	"internal/plangate:Defaults.actions":               {diag.CategoryPlan, diag.KeyPlanPolicyActions},
	"internal/plangate:Defaults.authoring_policy":      {diag.CategoryPlan, diag.KeyPlanPolicyAuthoring},
	"internal/plangate:TypeDefaults.name":              {diag.CategoryPlan, diag.KeyPlanPolicyTypes},
	"internal/plangate:TypeDefaults.model":             {diag.CategoryPlan, diag.KeyPlanPolicyTypeModels},
	"internal/plangate:TypeDefaults.tools":             {diag.CategoryPlan, diag.KeyPlanPolicyTools},
	"internal/plangate:TypeDefaults.actions":           {diag.CategoryPlan, diag.KeyPlanPolicyActions},

	"internal/project:fileConfig.models":        {diag.CategoryModel, diag.KeyModelVariants},
	"internal/project:fileConfig.skill_path":    {diag.CategoryContext, diag.KeyContextSkills},
	"internal/project:fileConfig.permissions":   {diag.CategoryPermissions, diag.KeyPermissionMode},
	"internal/project:fileConfig.agents":        {diag.CategoryAgents, diag.KeyAgentsState},
	"internal/project:fileConfig.notifications": {diag.CategoryUI, diag.KeyNotifyMode},
	"internal/project:fileConfig.opencode":      {diag.CategoryModel, diag.KeyModelImport},
	"internal/project:fileConfig.voice":         {diag.CategoryUI, diag.KeyVoiceState},
	"internal/project:fileConfig.keybinds":      {diag.CategoryUI, diag.KeyKeybindsOverrides},

	"internal/project:modelEntry.name":              {diag.CategoryModel, diag.KeyModelName},
	"internal/project:modelEntry.api_name":          {diag.CategoryModel, diag.KeyModelRequestName},
	"internal/project:modelEntry.provider":          {diag.CategoryModel, diag.KeyModelProvider},
	"internal/project:modelEntry.protocol":          {diag.CategoryModel, diag.KeyModelProtocol},
	"internal/project:modelEntry.api_key":           {diag.CategoryModel, diag.KeyModelCredential},
	"internal/project:modelEntry.context_window":    {diag.CategoryModel, diag.KeyModelContextWindow},
	"internal/project:modelEntry.max_output_tokens": {diag.CategoryModel, diag.KeyModelMaxOutputTokens},
	"internal/project:modelEntry.reasoning_effort":  {diag.CategoryModel, diag.KeyModelEffort},
	"internal/project:modelEntry.default":           {diag.CategoryModel, diag.KeyModelName},

	"internal/project:permConfig.mode":                  {diag.CategoryPermissions, diag.KeyPermissionMode},
	"internal/project:permConfig.workspace_only_writes": {diag.CategoryPermissions, diag.KeyPermissionWrites},
	"internal/project:permConfig.ask_timeout_sec":       {diag.CategoryPermissions, diag.KeyPermissionAskTimeout},
	"internal/project:permConfig.dangerously_allow_all": {diag.CategoryPermissions, diag.KeyPermissionBypass},
	"internal/project:permConfig.bash":                  {diag.CategoryPermissions, diag.KeyPermissionBashDefault},
	"internal/project:permConfig.mcp":                   {diag.CategoryPermissions, diag.KeyPermissionMCPAllow},
	"internal/project:permConfig.tasks":                 {diag.CategoryPermissions, diag.KeyPermissionTasks},
	"internal/project:bashConfig.default":               {diag.CategoryPermissions, diag.KeyPermissionBashDefault},
	"internal/project:bashConfig.allow":                 {diag.CategoryPermissions, diag.KeyPermissionBashAllow},
	"internal/project:bashConfig.deny":                  {diag.CategoryPermissions, diag.KeyPermissionBashDeny},
	"internal/project:mcpConfig.allow":                  {diag.CategoryPermissions, diag.KeyPermissionMCPAllow},

	"internal/project:fileConfig.web":                       {diag.CategoryIntegrations, diag.KeyWebState},
	"internal/project:webFileConfig.enabled":                {diag.CategoryIntegrations, diag.KeyWebState},
	"internal/project:webFileConfig.quarantine":             {diag.CategoryIntegrations, diag.KeyWebQuarantine},
	"internal/project:webFileConfig.cache_dir":              {diag.CategoryIntegrations, diag.KeyWebCache},
	"internal/project:webFileConfig.allowed_schemes":        {diag.CategoryIntegrations, diag.KeyWebSchemes},
	"internal/project:webFileConfig.allowed_hosts":          {diag.CategoryIntegrations, diag.KeyWebHosts},
	"internal/project:webFileConfig.denied_hosts":           {diag.CategoryIntegrations, diag.KeyWebHosts},
	"internal/project:webFileConfig.max_source_bytes":       {diag.CategoryIntegrations, diag.KeyWebFetch},
	"internal/project:webFileConfig.timeout_seconds":        {diag.CategoryIntegrations, diag.KeyWebFetch},
	"internal/project:webFileConfig.max_redirects":          {diag.CategoryIntegrations, diag.KeyWebFetch},
	"internal/project:webFileConfig.accepted_content_types": {diag.CategoryIntegrations, diag.KeyWebFetch},
	"internal/project:webFileConfig.user_agent":             {diag.CategoryIntegrations, diag.KeyWebFetch},
	"internal/project:webFileConfig.search_provider":        {diag.CategoryIntegrations, diag.KeyWebSearch},
	"internal/project:webFileConfig.max_search_results":     {diag.CategoryIntegrations, diag.KeyWebSearch},
	"internal/project:webFileConfig.search_url":             {diag.CategoryIntegrations, diag.KeyWebSearch},
	"internal/project:webFileConfig.google_cse_url":         {diag.CategoryIntegrations, diag.KeyWebSearch},
	"internal/project:webFileConfig.google_cse_id":          {diag.CategoryIntegrations, diag.KeyWebSearch},
	"internal/project:webFileConfig.google_api_key_env":     {diag.CategoryIntegrations, diag.KeyWebCredential},
	"internal/project:webFileConfig.allow":                  {diag.CategoryPermissions, diag.KeyPermissionWebAllow},

	"internal/project:agentsConfig.enabled":          {diag.CategoryAgents, diag.KeyAgentsState},
	"internal/project:agentsConfig.models":           {diag.CategoryAgents, diag.KeyAgentsModels},
	"internal/project:openCodeFileConfig.enabled":    {diag.CategoryModel, diag.KeyModelImport},
	"internal/project:notificationsFileConfig.mode":  {diag.CategoryUI, diag.KeyNotifyMode},
	"internal/project:notificationsFileConfig.sound": {diag.CategoryUI, diag.KeyNotifySound},

	"internal/project:UIState.planDisabled": {diag.CategoryPlan, diag.KeyPlanEnabled},
	"internal/project:UIState.lastModel":    {diag.CategoryModel, diag.KeyModelName},
	"internal/project:UIState.lastEffort":   {diag.CategoryModel, diag.KeyModelEffort},
	"internal/project:UIState.editingMode":  {diag.CategoryUI, diag.KeyKeymapMode},

	"internal/session:PlanAction.type":           {diag.CategoryPlan, diag.KeyPlanPolicyActions},
	"internal/session:PlanAction.event":          {diag.CategoryPlan, diag.KeyPlanPolicyActions},
	"internal/session:PlanAction.runs":           {diag.CategoryPlan, diag.KeyPlanStepActions},
	"internal/session:PlanAction.skills":         {diag.CategoryPlan, diag.KeyPlanStepSkills},
	"internal/session:PlanAction.disabledSkills": {diag.CategoryPlan, diag.KeyPlanStepSkills},

	"internal/tasks:cfg.task_registry":               {diag.CategoryStorage, diag.KeyTasksState},
	"internal/tasks:cfg.task_registry.backend":       {diag.CategoryStorage, diag.KeyTasksState},
	"internal/tasks:cfg.task_registry.obsidian":      {diag.CategoryStorage, diag.KeyTasksLocation},
	"internal/tasks:cfg.task_registry.obsidian.path": {diag.CategoryStorage, diag.KeyTasksLocation},

	"internal/voice:FileConfig.enabled":    {diag.CategoryUI, diag.KeyVoiceState},
	"internal/voice:FileConfig.capture":    {diag.CategoryUI, diag.KeyVoiceCapture},
	"internal/voice:FileConfig.stt":        {diag.CategoryUI, diag.KeyVoiceBackend},
	"internal/voice:STTFileConfig.backend": {diag.CategoryUI, diag.KeyVoiceBackend},
	"internal/voice:STTFileConfig.model":   {diag.CategoryUI, diag.KeyVoiceModel},
	"internal/voice:STTFileConfig.api_key": {diag.CategoryUI, diag.KeyVoiceCredential},
}

var withheldConfig = merge(
	because(
		"адрес провайдера умеет нести токен в пути или query, поэтому эндпоинт не выводится ни одним "+
			"слоем; кто провайдер и есть ли учётные данные, видно в model.provider и model.credential",
		"internal/project:modelEntry.base_url",
	),
	because(
		"адрес и командная строка распознавания: первый может нести токен, вторая — исполняемая строка "+
			"пользователя; в каталоге виден вид бэкенда (ui.voice.backend) и наличие ключа (ui.voice.credential)",
		"internal/voice:STTFileConfig.base_url",
		"internal/voice:STTFileConfig.command",
	),
	because(
		"командная строка захвата и имя аудиоустройства принадлежат машине пользователя; в каталоге видно "+
			"только, свой это захват или пресет (ui.voice.capture)",
		"internal/voice:CaptureFileConfig.command",
		"internal/voice:CaptureFileConfig.device",
	),
	because(
		"ключ снят: загрузка отвергает его отдельной ошибкой, поэтому настройкой он больше не является",
		"internal/voice:FileConfig.auto_send",
	),
	because(
		"ключ снят: загрузка отвергает литерал и подставляет пустую строку, поэтому настройкой он больше "+
			"не является; есть ли у поиска учётные данные, видно в integrations.web.search.credential",
		"internal/project:webFileConfig.google_api_key",
	),
	because(
		"геометрия и история панелей TUI: раскладка, а не поведение harness — каталог описывает, что harness "+
			"делает, а не как он выглядит",
		"internal/project:UIState.sidebarWidth",
		"internal/project:UIState.sidebarHidden",
		"internal/project:UIState.expandEditsDisabled",
		"internal/project:UIState.statusCloses",
	),
)

var missingConfig = merge(
	gaps(ticketSettingTail,
		"настройка меняет поведение сессии, но не отражена ни одним полем каталога",
		"internal/voice:FileConfig.language",
		"internal/voice:FileConfig.auto_pause_seconds",
		"internal/voice:FileConfig.max_seconds",
		"internal/voice:FileConfig.segment_silence_ms",
		"internal/voice:FileConfig.hints",
		"internal/voice:FileConfig.glossary",
		"internal/voice:STTFileConfig.provider",
		"internal/voice:STTFileConfig.timeout_seconds",
		"internal/project:UIState.stopLimitDisabled",
	),
)

// envCoverage accounts for every environment variable a loader reads.
var envCoverage = coverageTable{
	kind: surfaceEnv,
	reported: map[string]catalogRef{
		"COZYPHI_API_KEY":    {diag.CategoryModel, diag.KeyModelCredential},
		"COZYPHI_MODEL":      {diag.CategoryModel, diag.KeyModelName},
		"COZYPHI_SKILL_PATH": {diag.CategoryContext, diag.KeyContextSkills},
		"COZYPHI_DEBUG":      {diag.CategoryDiagnostics, diag.KeyLoggingState},
		"COZYPHI_DEBUG_FILE": {diag.CategoryDiagnostics, diag.KeyLoggingDestination},
		"COZYPHI_PPROF":      {diag.CategoryDiagnostics, diag.KeyProfilingState},
		"COZYPHI_HOOKS":      {diag.CategoryIntegrations, diag.KeyHooksState},
		"COZYPHI_MCP":        {diag.CategoryIntegrations, diag.KeyMCPDisabled},
	},
	withheld: withheldEnv,
	missing: gaps(ticketSettingTail,
		"переменная включает отдельный журнал подсистемы, и ни включённость, ни назначение журнала "+
			"не отражены в каталоге",
		"COZYPHI_MCP_LOG_DIR",
		"COZYPHI_PLAN_GATE_LOG_DIR",
	),
}

var withheldEnv = merge(
	because(
		"адрес провайдера не выводится ни одним слоем; то, что источником оказалось окружение, видно "+
			"в model.source_order",
		"COZYPHI_BASE_URL",
	),
	because(
		"относится к проверке обновлений, которая живёт вне сессии; наблюдаемая поверхность — работающая "+
			"сессия, а не апдейтер",
		"COZYPHI_OFFLINE",
		"COZYPHI_SKIP_VERSION_CHECK",
		"GITHUB_TOKEN",
	),
	because(
		"определяет, откуда читается импорт opencode; сам импорт и его результат видны в "+
			"model.import.opencode и model.import.opencode.models",
		"OPENCODE_CONFIG",
		"OPENCODE_CONFIG_DIR",
		"XDG_CONFIG_HOME",
		"XDG_DATA_HOME",
	),
	because(
		"переменная окружения ОС, а не настройка cozyphi: что из языковых серверов найдено на этой машине, "+
			"видно в integrations.lsp.servers",
		"PATH",
	),
)

// flagCoverage accounts for every command line flag cmd parses.
var flagCoverage = coverageTable{
	kind: surfaceFlag,
	reported: map[string]catalogRef{
		"--developer-mode": {diag.CategoryRuntime, diag.KeyDeveloperMode},
		"--continue":       {diag.CategoryRuntime, diag.KeySessionID},
		"--continue-last":  {diag.CategoryRuntime, diag.KeySessionID},
		"--resume":         {diag.CategoryRuntime, diag.KeySessionID},
		"--session":        {diag.CategoryRuntime, diag.KeySessionID},
		"--session-dir":    {diag.CategoryStorage, diag.KeySessionsLocation},
		"--yolo":           {diag.CategoryPermissions, diag.KeyPermissionBypass},
	},
	withheld: merge(
		because(
			"печатает справку и завершает процесс: сессии, которую можно было бы наблюдать, не возникает",
			"--help",
			"--check",
		),
		because(
			"текст запроса пользователя, а не настройка; содержимое запроса эпик выводить запрещает",
			"--prompt",
		),
	),
	missing: gaps(ticketSettingTail,
		"ограничение или формат вывода headless-запуска, не отражённые ни одним полем каталога",
		"--jsonl",
		"--max-rounds",
		"--timeout",
	),
}

// runtimeCoverage is the explicit list the fixed decisions on this ticket ask
// for: what acts on a session without being written down anywhere a scan can
// reach — a limit compiled in, an override a slash command applies for one
// session, a ceiling a parent puts on a child at spawn.
var runtimeCoverage = coverageTable{
	kind: surfaceRuntime,
	reported: map[string]catalogRef{
		"answer size and time limits":   {diag.CategoryDiagnostics, diag.KeyHarnessLimits},
		"watch limits":                  {diag.CategoryDiagnostics, diag.KeyWatchesLimits},
		"sub-agent nesting ceiling":     {diag.CategoryAgents, diag.KeyAgentsDepth},
		"sub-agent concurrency ceiling": {diag.CategoryAgents, diag.KeyAgentsConcurrency},
		"spawn role and depth":          {diag.CategoryAgents, diag.KeyAgentsRoles},
		"hook timeout":                  {diag.CategoryIntegrations, diag.KeyHooksTimeout},
		"lsp start policy":              {diag.CategoryIntegrations, diag.KeyLSPStart},
		"lsp operation set":             {diag.CategoryIntegrations, diag.KeyLSPOperations},
		"compaction headroom":           {diag.CategoryContext, diag.KeyContextCompactHeadroom},
		"compaction keep_recent":        {diag.CategoryContext, diag.KeyContextKeepRecent},
		"workspace-only reads":          {diag.CategoryPermissions, diag.KeyPermissionReads},
		"sensitive path list":           {diag.CategoryPermissions, diag.KeyPermissionSensitive},
		"memory tool access":            {diag.CategoryPermissions, diag.KeyPermissionMemory},
		"session allow-all overlay":     {diag.CategoryPermissions, diag.KeyPermissionBypass},
		"/context window override":      {diag.CategoryContext, diag.KeyContextOverride},
		"/model session override":       {diag.CategoryModel, diag.KeyModelName},
		"/effort session override":      {diag.CategoryModel, diag.KeyModelEffort},
		"/theme session palette":        {diag.CategoryUI, diag.KeyThemeName},
		"plan model pin":                {diag.CategoryModel, diag.KeyModelPlanPinned},
	},
	withheld: because(
		"поиск bash на Windows идёт по переменным окружения ОС (ProgramFiles): свойство машины, а не "+
			"настройка cozyphi",
		"windows bash discovery",
	),
}

// because records one justification against every setting it covers.
func because(why string, ids ...string) map[string]string {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		out[id] = why
	}
	return out
}

// gaps records one filed ticket against every setting waiting on it.
func gaps(ticket, why string, ids ...string) map[string]gap {
	out := make(map[string]gap, len(ids))
	for _, id := range ids {
		out[id] = gap{ticket: ticket, why: why}
	}
	return out
}

// merge joins the groups of one column into the column itself. A setting
// named twice is a mistake in the inventory rather than an override, so it
// panics rather than letting the second entry win silently.
func merge[V any](groups ...map[string]V) map[string]V {
	out := map[string]V{}
	for _, group := range groups {
		for id, value := range group {
			if _, clash := out[id]; clash {
				panic("coverage inventory: " + id + " is accounted for twice")
			}
			out[id] = value
		}
	}
	return out
}

// The inventory is checked from both sides. On one side the source: every
// setting a loader accepts has to be accounted for, and every setting the
// inventory accounts for has to still exist. On the other side a live
// registry: every field the inventory points at has to be one the harness
// actually reports, in the category it says.

func TestEverySettingTheHarnessAcceptsIsAccountedFor(t *testing.T) {
	scanned := scanSettableSurface(t)
	used := map[string]bool{}

	for _, table := range inventory() {
		if table.kind == surfaceRuntime {
			// Runtime entries are the explicit list the fixed decisions ask
			// for: a compiled-in limit is in no loader for a scan to find.
			continue
		}
		t.Run(string(table.kind), func(t *testing.T) {
			var settable []string
			for _, id := range scanned.ids(table.kind) {
				root := rootOf(id)
				if _, skip := skippedRoots[root]; skip {
					used[root] = true
					continue
				}
				settable = append(settable, id)
			}
			assert.Empty(t, absent(settable, table.ids()),
				"the harness accepts these settings and the inventory accounts for none of them")
			assert.Empty(t, absent(table.ids(), settable),
				"the inventory accounts for these settings and no loader accepts them any more")
		})
	}

	for root, why := range skippedRoots {
		assert.True(t, used[root], "%s is skipped as %q and no longer exists", root, why)
	}
}

func TestEveryAccountedSettingNamesAFieldTheHarnessReports(t *testing.T) {
	catalog := liveCatalog(t)

	for _, table := range inventory() {
		for _, id := range slices.Sorted(maps.Keys(table.reported)) {
			ref := table.reported[id]
			keys, ok := catalog[ref.category]
			if !assert.True(t, ok, "%s is reported in category %s, which the harness does not have",
				id, ref.category) {
				continue
			}
			assert.Contains(t, keys, ref.key,
				"%s is reported as %s %s, which that category does not have", id, ref.category, ref.key)
		}
	}
}

// AC 2 of this ticket, asked of the wiring rather than of a fixture registry:
// the placeholder availability the first ticket introduced must be gone, and
// no category may be reported unavailable in a session where all eleven
// collectors are wired — an unavailable category is a real state, never a
// stand-in for a collector nobody connected.
func TestNoCategoryIsStillWaitingForItsCollector(t *testing.T) {
	c := developerToolRuntime(t)

	overview, err := c.diagnostics.Snapshot(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, overview.Categories, len(diag.Categories()))

	for _, entry := range overview.Categories {
		assert.Equal(t, diag.AvailabilityAvailable, entry.Availability,
			"%s answers %s (%s)", entry.Category, entry.Availability, entry.Reason)
	}

	for _, category := range diag.Categories() {
		detail, err := c.diagnostics.Snapshot(t.Context(), category)
		require.NoError(t, err)
		require.Len(t, detail.Categories, 1)
		assert.NotEqual(t, diag.AvailabilityNotImplemented, detail.Categories[0].Availability,
			"%s has no collector", category)
	}
}

// AC 7: a gap is a filed ticket. The inventory cannot record one without
// naming the ticket that closes it, and a ticket that stops being named is a
// constant nobody uses — both are failures here rather than quiet drift. The
// tickets themselves live in the epic's ledger and block its acceptance,
// which is what stops a gap from being a permanent justification.
func TestEveryGapNamesATicketThatBlocksTheEpic(t *testing.T) {
	filed := map[string]bool{ticketSettingTail: false}

	for _, table := range inventory() {
		for _, id := range slices.Sorted(maps.Keys(table.missing)) {
			entry := table.missing[id]
			assert.NotEmpty(t, entry.why, "%s is missing for no stated reason", id)
			if _, known := filed[entry.ticket]; !assert.True(t, known,
				"%s waits on %q, which is not a ticket of this epic", id, entry.ticket) {
				continue
			}
			filed[entry.ticket] = true
		}
	}

	for ticket, named := range filed {
		assert.True(t, named, "%s is filed and the inventory names no gap it closes", ticket)
	}
}

// liveCatalog is what a granted session says it can report, by category.
func liveCatalog(t *testing.T) map[diag.Category][]string {
	t.Helper()
	c := developerToolRuntime(t)
	byCategory := map[diag.Category][]string{}
	for _, entry := range c.diagnostics.Catalog().Categories {
		byCategory[entry.Category] = entry.Keys
	}
	return byCategory
}

// rootOf is the schema a setting belongs to: the package and the struct or
// file it is declared in, without the field path below it.
func rootOf(id string) string {
	pkg, rest, ok := strings.Cut(id, ":")
	if !ok {
		return id
	}
	owner, _, _ := strings.Cut(rest, ".")
	return pkg + ":" + owner
}

// absent returns the members of want that have nothing in have to match them.
func absent(want, have []string) []string {
	var out []string
	for _, id := range want {
		if !slices.Contains(have, id) {
			out = append(out, id)
		}
	}
	return out
}
