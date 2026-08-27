package watch

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	// maxLineBytes caps one line before it is emitted. A watch pointed at a
	// binary file must not buffer without bound waiting for a newline.
	maxLineBytes = 8 * 1024
	// exitTailLines is how much of a finished command's output comes back with
	// its exit code — enough to see why it failed, not the whole run.
	exitTailLines = 20
	// pollOutputLimit bounds what one poll run collects for comparison.
	pollOutputLimit = 64 * 1024
)

// Source produces events until its context ends. Returning nil means the
// source finished on its own; an error means it broke and says how.
type Source interface {
	Run(ctx context.Context, emit func(text string)) error
}

// newSource picks the shape from an already normalized spec.
func newSource(spec Spec, match *regexp.Regexp, shell ShellFunc) Source {
	if spec.Every > 0 {
		return &ticker{
			label:   spec.Label,
			command: spec.Command,
			match:   match,
			every:   spec.Every,
			once:    spec.Once,
			shell:   shell,
		}
	}
	return &stream{command: spec.Command, on: spec.On, match: match, shell: shell}
}

// stream runs one command and turns its output into events: a line at a time
// while it runs, or a single event when it exits.
type stream struct {
	command string
	on      Trigger
	match   *regexp.Regexp
	shell   ShellFunc
}

func (s *stream) Run(ctx context.Context, emit func(string)) error {
	tail := newRing(exitTailLines)
	lines := &lineSplitter{fn: func(line string) {
		if s.match != nil && !s.match.MatchString(line) {
			return
		}
		if s.on == OnExit {
			tail.add(line)
			return
		}
		emit(line)
	}}

	res, err := s.shell(ctx, s.command, lines.write)
	lines.close()
	if err != nil {
		return err
	}
	if s.on != OnExit {
		return nil
	}
	if res.Canceled {
		return context.Canceled
	}
	emit(exitReport(s.command, res.ExitCode, tail.lines()))
	return nil
}

// exitReport is what a finished command comes back with: the verdict first,
// because that is what the reader acts on, then the tail that explains it.
func exitReport(command string, code int, tail []string) string {
	verdict := "succeeded"
	if code != 0 {
		verdict = fmt.Sprintf("failed with exit %d", code)
	}
	report := fmt.Sprintf("%s %s", command, verdict)
	if len(tail) > 0 {
		report += "\n\n" + strings.Join(tail, "\n")
	}
	return report
}

// ticker fires on an interval. With a command it polls: the first run is the
// baseline and is silent, and every run whose output differs from the last one
// is an event. Without a command it is a plain reminder that says its label.
type ticker struct {
	label   string
	command string
	match   *regexp.Regexp
	every   time.Duration
	once    bool
	shell   ShellFunc
}

func (t *ticker) Run(ctx context.Context, emit func(string)) error {
	polling := t.command != ""
	previous, baseline := "", false

	// A poll runs at once to learn what "unchanged" means; a bare reminder
	// waits out its interval, because a 20-minute reminder that fires now is
	// not a reminder.
	if polling {
		out, err := t.poll(ctx)
		if err != nil {
			return err
		}
		previous, baseline = out, true
		if t.once {
			emit(out)
			return nil
		}
	}

	tick := time.NewTicker(t.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if !polling {
				emit(t.label)
				if t.once {
					return nil
				}
				continue
			}
			out, err := t.poll(ctx)
			if err != nil {
				return err
			}
			if baseline && out != previous {
				emit(out)
			}
			previous, baseline = out, true
		}
	}
}

// poll runs the command once and returns the filtered output one comparison
// is made against.
func (t *ticker) poll(ctx context.Context) (string, error) {
	var collected strings.Builder
	lines := &lineSplitter{fn: func(line string) {
		if t.match != nil && !t.match.MatchString(line) {
			return
		}
		if collected.Len() >= pollOutputLimit {
			return
		}
		collected.WriteString(line)
		collected.WriteByte('\n')
	}}
	if _, err := t.shell(ctx, t.command, lines.write); err != nil {
		return "", err
	}
	lines.close()
	return strings.TrimRight(collected.String(), "\n"), nil
}

// lineSplitter turns streamed chunks into whole lines. The process writes
// stdout and stderr through one pipe, so this is only ever called from one
// goroutine and needs no lock.
type lineSplitter struct {
	buf strings.Builder
	fn  func(string)
}

func (s *lineSplitter) write(chunk string) {
	for chunk != "" {
		newline := strings.IndexByte(chunk, '\n')
		if newline < 0 {
			s.appendBounded(chunk)
			return
		}
		s.appendBounded(chunk[:newline])
		s.flush()
		chunk = chunk[newline+1:]
	}
}

// appendBounded keeps a line under maxLineBytes, dropping the overflow rather
// than growing: what is past the cap is not what identified the event.
func (s *lineSplitter) appendBounded(text string) {
	room := maxLineBytes - s.buf.Len()
	if room <= 0 {
		return
	}
	if len(text) > room {
		text = text[:room]
	}
	s.buf.WriteString(text)
}

func (s *lineSplitter) flush() {
	line := strings.TrimRight(s.buf.String(), "\r")
	s.buf.Reset()
	if strings.TrimSpace(line) != "" {
		s.fn(line)
	}
}

// close flushes a final line the command left without a newline.
func (s *lineSplitter) close() { s.flush() }

// ring keeps the newest n lines.
type ring struct {
	items []string
	n     int
}

func newRing(n int) *ring { return &ring{n: n} }

func (r *ring) add(line string) {
	r.items = append(r.items, line)
	if len(r.items) > r.n {
		r.items = r.items[len(r.items)-r.n:]
	}
}

func (r *ring) lines() []string { return r.items }
