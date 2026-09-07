package debuglog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// latched reports the package's own state, which is the thing an observation
// must not change.
func latched() (checkedNow, enabledNow bool) {
	mu.Lock()
	defer mu.Unlock()
	return checked, enabled
}

// Reading the harness must not change what the harness reports. Enabled
// latches the switch on its first call, so Observe reads the cached answer
// and the environment beside it rather than asking.
func TestObservingTheLogDoesNotLatchTheSwitch(t *testing.T) {
	t.Setenv("COZYPHI_DEBUG", "1")
	t.Setenv("COZYPHI_DEBUG_FILE", filepath.Join(t.TempDir(), "debug.log"))
	resetState(t)

	facts := Observe()
	if !facts.Known {
		t.Fatal("the layer is wired, so it is known")
	}
	if !facts.Requested {
		t.Fatal("the environment says on and the observation must say so")
	}
	if facts.Latched {
		t.Fatal("nothing has read the switch yet")
	}
	if checkedNow, _ := latched(); checkedNow {
		t.Fatal("observing must not latch the switch it is reporting")
	}

	if !Enabled() {
		t.Fatal("the switch reads on once something asks")
	}
	after := Observe()
	if !after.Latched || !after.Enabled {
		t.Fatalf("after the first read the switch is latched on, got %+v", after)
	}
	if after.Revision == facts.Revision {
		t.Fatal("two answers across the first read are answers about two different states")
	}
}

// The switch is read once and remembered, so an environment changed
// afterwards is configured one way and acting another. That gap is the usual
// explanation for a log that is "on" and empty, and it needs both answers.
func TestTheSwitchAndTheAnswerItLatchedToCanDisagree(t *testing.T) {
	t.Setenv("COZYPHI_DEBUG", "1")
	t.Setenv("COZYPHI_DEBUG_FILE", filepath.Join(t.TempDir(), "debug.log"))
	resetState(t)
	_ = Enabled()

	t.Setenv("COZYPHI_DEBUG", "")
	facts := Observe()
	if facts.Requested {
		t.Fatal("the environment says off now")
	}
	if !facts.Latched || !facts.Enabled {
		t.Fatalf("and the answer it was read into still says on, got %+v", facts)
	}
}

// Only the name travels. Where the file sits is a home directory or a path a
// script chose, and neither is needed to answer the question.
func TestTheDestinationTravelsAsANameAndNeverAsAPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COZYPHI_DEBUG", "1")
	t.Setenv("COZYPHI_DEBUG_FILE", filepath.Join(dir, "debug.log"))
	resetState(t)

	Logf("something to say")
	facts := Observe()

	if facts.Destination != "debug.log" || facts.OpenName != "debug.log" {
		t.Fatalf("the base name is the whole answer, got %+v", facts)
	}
	if !facts.FromEnv {
		t.Fatal("the environment named this file and the observation says which layer did")
	}
	for _, carried := range []string{facts.Destination, facts.OpenName, facts.Revision} {
		if strings.Contains(carried, dir) || strings.Contains(carried, string(os.PathSeparator)) {
			t.Fatalf("no part of the directory may travel, got %q", carried)
		}
	}
}

// The file is opened by the first line written rather than at startup, so a
// log that is on and has had nothing to say has a destination and no open
// file — which is not a destination that could not be opened.
func TestAFileIsOpenedByTheFirstLineAndNotBefore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("COZYPHI_DEBUG", "1")
	t.Setenv("COZYPHI_DEBUG_FILE", path)
	resetState(t)

	if quiet := Observe(); quiet.OpenName != "" {
		t.Fatalf("nothing has been written, so no file is open, got %+v", quiet)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("observing the log must not create it (err %v)", err)
	}

	Logf("the first line")
	if written := Observe(); written.OpenName != "debug.log" {
		t.Fatalf("the first line opens the file, got %+v", written)
	}
}

// A default destination is a default, and the observation says so rather
// than implying somebody chose it.
func TestADefaultDestinationIsNotReportedAsAChoice(t *testing.T) {
	t.Setenv("COZYPHI_DEBUG", "")
	t.Setenv("COZYPHI_DEBUG_FILE", "   ")
	resetState(t)

	facts := Observe()
	if facts.Destination != defaultPath {
		t.Fatalf("with nothing named, lines would land on the default, got %q", facts.Destination)
	}
	if facts.FromEnv {
		t.Fatal("nothing named it")
	}
}

// The audit line is one line in this log and nothing else: no file, no
// stream, no second destination somebody has to know about.
func TestAHarnessRecordIsOneLineInThisLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("COZYPHI_DEBUG", "1")
	t.Setenv("COZYPHI_DEBUG_FILE", path)
	resetState(t)

	AuditSink(diag.AuditEvent{
		Action:   diag.AuditSnapshot,
		Category: diag.CategoryRuntime,
		Mode:     "detail",
		Result:   diag.AuditAnswered,
		Fields:   3,
	})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the log file must exist: %v", err)
	}
	body := string(raw)
	for _, want := range []string{"harness snapshot", "category=runtime", "result=answered", "fields=3"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the record must carry %q, got %q", want, body)
		}
	}
}

// With the switch off a harness request writes nothing at all: the record is
// a debug line like any other, and the default path sits in the working
// directory.
func TestAHarnessRecordIsWrittenNowhereWhenTheLogIsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("COZYPHI_DEBUG", "")
	t.Setenv("COZYPHI_DEBUG_FILE", path)
	resetState(t)

	AuditSink(diag.AuditEvent{Action: diag.AuditCatalog, Result: diag.AuditAnswered})

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a request must not create the log (err %v)", err)
	}
}

// The log is observed, never read. A debug line quotes whatever the process
// was doing at the time, so what has been written stays in the file and
// nothing in the observation can carry it out.
func TestNothingWrittenToTheLogTravelsWithTheObservation(t *testing.T) {
	const sentinel = "sentinel-line-nothing-may-carry-out"
	path := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("COZYPHI_DEBUG", "1")
	t.Setenv("COZYPHI_DEBUG_FILE", path)
	resetState(t)

	Logf("api key %s and a whole conversation besides", sentinel)

	facts := Observe()
	rendered := fmt.Sprintf("%+v", facts)
	if strings.Contains(rendered, sentinel) {
		t.Fatalf("no part of the log may travel, got %q", rendered)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the line must still be in the file: %v", err)
	}
	if !strings.Contains(string(raw), sentinel) {
		t.Fatal("the line was written; the observation simply does not read it")
	}
}

// The two subsystem logs honor a directory of their own, read from the
// environment. Whether each is redirected travels; the directory — a home
// directory or a path a script chose — is planted here and must go nowhere.
func TestARedirectedSubsystemLogTravelsAsAFactAndNeverAsItsDirectory(t *testing.T) {
	t.Setenv("COZYPHI_DEBUG", "")
	t.Setenv("COZYPHI_DEBUG_FILE", "")
	mcpDir := filepath.Join(t.TempDir(), "secret-mcp-logs")
	t.Setenv("COZYPHI_MCP_LOG_DIR", mcpDir)
	t.Setenv("COZYPHI_PLAN_GATE_LOG_DIR", "   ")
	resetState(t)

	facts := Observe()
	if !facts.MCPLogFromEnv {
		t.Fatal("the environment redirects the MCP server logs and the observation says so")
	}
	if facts.PlanGateLogFromEnv {
		t.Fatal("a blank value is unset, as the plan gate itself reads it")
	}
	if !strings.Contains(facts.Revision, ".m1") || !strings.Contains(facts.Revision, ".p0") {
		t.Fatalf("the fingerprint must tell the redirects apart, got %q", facts.Revision)
	}
	for _, carried := range []string{facts.Destination, facts.OpenName, facts.Revision} {
		if strings.Contains(carried, "secret-mcp-logs") || strings.Contains(carried, string(os.PathSeparator)) {
			t.Fatalf("no part of the directory may travel, got %q", carried)
		}
	}
	if fmt.Sprint(facts) != strings.ReplaceAll(fmt.Sprint(facts), mcpDir, "") {
		t.Fatalf("the directory is nowhere in the observation, got %+v", facts)
	}

	t.Setenv("COZYPHI_PLAN_GATE_LOG_DIR", filepath.Join(t.TempDir(), "gate"))
	both := Observe()
	if !both.PlanGateLogFromEnv || both.Revision == facts.Revision {
		t.Fatalf("redirecting the plan-gate log is a change of state, got %+v", both)
	}
}
