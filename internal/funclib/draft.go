package funclib

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/tools/writetool"
	"github.com/alvnukov/cozyphi/internal/util"
)

func (s *Store) getDraft(ctx context.Context, session, name string) (string, *snapshot, error) {
	ref, err := draftRef(session, name)
	if err != nil {
		return "", nil, err
	}
	oid, err := s.ref(ctx, ref)
	if err != nil {
		return ref, nil, err
	}
	if oid == "" {
		return ref, nil, errors.New("open the package draft in this session first")
	}
	sn, err := s.readSnapshot(ctx, oid)
	return ref, sn, err
}

// DraftState reads a draft without creating one.
func (s *Store) DraftState(ctx context.Context, session, name string) (Package, error) {
	_, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return Package{}, err
	}
	return sn.pkg(), nil
}

// Each incarnation gets fresh grants even when its source matches a retired
// draft byte for byte after publication or rebase.
func draftFileKey(ref string, sn *snapshot, file string) string {
	return ref + "/" + sn.id.Generation + "/" + file
}

// Draft opens the same private revision again after restart.
// A new draft is based on the requested release, or the latest release.
func (s *Store) Draft(ctx context.Context, session, name, version string) (Package, error) {
	ref, err := draftRef(session, name)
	if err != nil {
		return Package{}, err
	}
	if version != "" && !versionOK(version) {
		return Package{}, errors.New("provide an exact semantic version")
	}
	if err := s.ensure(ctx); err != nil {
		return Package{}, err
	}
	existing, err := s.ref(ctx, ref)
	if err != nil {
		return Package{}, err
	}
	if existing != "" {
		sn, err := s.readSnapshot(ctx, existing)
		if err != nil {
			return Package{}, err
		}
		return sn.pkg(), nil
	}
	baseRef := branch(name)
	if version != "" {
		baseRef = tag(name, version)
	}
	base, err := s.ref(ctx, baseRef)
	if err != nil {
		return Package{}, err
	}
	if base == "" && version != "" {
		return Package{}, fmt.Errorf("release %s@%s does not exist", name, version)
	}
	sn := &snapshot{
		id:    identity{Name: name, Base: base, SDK: sdkVersion, Generation: rand.Text()},
		files: make(map[string]string),
	}
	if base != "" {
		prev, err := s.readSnapshot(ctx, base)
		if err != nil {
			return Package{}, err
		}
		sn.files = maps.Clone(prev.files)
	}
	sn.digest, err = s.commit(ctx, sn, "open "+name+" draft", base)
	if err != nil {
		return Package{}, err
	}
	if err := s.move(ctx, ref, sn.digest, ""); err != nil {
		_, other, readErr := s.getDraft(ctx, session, name)
		if readErr == nil {
			return other.pkg(), nil
		}
		return Package{}, err
	}
	return sn.pkg(), nil
}

func (s *Store) Get(ctx context.Context, name, version string) (Package, error) {
	oid, err := s.getOID(ctx, name, version)
	if err != nil {
		return Package{}, err
	}
	sn, err := s.readSnapshot(ctx, oid)
	if err != nil {
		return Package{}, err
	}
	if sn.id.Name != name || sn.id.Version == "" || (version != "" && sn.id.Version != version) {
		return Package{}, errors.New("package identity does not match its ref")
	}
	return sn.pkg(), nil
}

func (s *Store) getOID(ctx context.Context, name, version string) (string, error) {
	if !nameOK(name) {
		return "", errors.New("invalid package name")
	}
	ref := branch(name)
	if version != "" {
		if !versionOK(version) {
			return "", errors.New("provide an exact semantic version")
		}
		ref = tag(name, version)
	}
	yes, err := s.exists()
	if err != nil {
		return "", err
	}
	if !yes {
		return "", fmt.Errorf("package %s is not published", name)
	}
	oid, err := s.ref(ctx, ref)
	if err != nil {
		return "", err
	}
	if oid == "" {
		return "", fmt.Errorf("package %s or its requested version is not published", name)
	}
	return oid, nil
}

func (s *Store) save(ctx context.Context, ref string, sn *snapshot, file, content string) error {
	if len(content) > fileLimit {
		return errors.New("library file exceeds 256 KiB")
	}
	next := *sn
	next.files = maps.Clone(sn.files)
	next.files[file] = content
	oid, err := s.commit(ctx, &next, "edit "+sn.id.Name+"/"+file, sn.digest)
	if err != nil {
		return err
	}
	return s.move(ctx, ref, oid, sn.digest)
}

func rendered(file, content string, offset, limit int) (string, []string, error) {
	lines := strings.Split(content, "\n")
	offset = max(1, offset)
	if offset > len(lines) {
		return "", nil, errors.New("offset exceeds the file's line count")
	}
	if limit <= 0 || limit > 250 {
		limit = 250
	}
	rev := util.RevisionOf(content)
	var body strings.Builder
	body.WriteString(util.FormatFileHeader(file, rev.Tag()))
	var anchors []string
	for i := offset - 1; i < len(lines) && len(anchors) < limit; i++ {
		if len(lines[i]) > 20000 {
			if len(anchors) == 0 {
				return "", nil, errors.New("source line exceeds 20000 bytes; split long lines before creating the file")
			}
			break
		}
		if body.Len()+len(lines[i])+24 > 32768 {
			break
		}
		a := fmt.Sprintf("%d#%s", i+1, util.ComputeLineHash(lines[i]))
		anchors = append(anchors, a)
		fmt.Fprintf(&body, "\n%s|%s", a, lines[i])
	}
	if offset-1+len(anchors) < len(lines) {
		fmt.Fprintf(&body, "\n[more at line %d]", offset+len(anchors))
	}
	return body.String(), anchors, nil
}

func validFile(file string) error {
	if !slices.Contains(editableFiles, file) {
		return errors.New("file must be index.ts, index.test.ts or package.json")
	}
	return nil
}

func validText(content string) error {
	if len(content) > fileLimit {
		return errors.New("source exceeds the 256 KiB package file limit")
	}
	for line := range strings.SplitSeq(content, "\n") {
		if len(line) > 20000 {
			return errors.New("split source lines longer than 20000 bytes")
		}
	}
	return nil
}

func (s *Store) ReadEditable(ctx context.Context, session, name, file string) (string, error) {
	return s.ReadRange(ctx, session, name, file, 1, 0)
}

func (s *Store) ReadRange(ctx context.Context, session, name, file string, offset, limit int) (string, error) {
	if err := validFile(file); err != nil {
		return "", err
	}
	ref, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return "", err
	}
	raw, ok := sn.files[file]
	if !ok {
		return "", fmt.Errorf("%s is absent; create it first: %w", file, os.ErrNotExist)
	}
	body, anchors, err := rendered(file, raw, offset, limit)
	if err != nil {
		return "", err
	}
	s.ledger.Authorize(draftFileKey(ref, sn, file), util.RevisionOf(raw), anchors)
	return body, nil
}

func (s *Store) CreateFile(ctx context.Context, session, name, file, content string) (string, error) {
	if err := validFile(file); err != nil {
		return "", err
	}
	content = util.NormalizeLF(content)
	if err := validText(content); err != nil {
		return "", err
	}
	ref, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return "", err
	}
	if _, ok := sn.files[file]; ok {
		return "", errors.New("file already exists; use editable read and hashline edit")
	}
	body, anchors, err := rendered(file, content, 1, 0)
	if err != nil {
		return "", err
	}
	if err := s.save(ctx, ref, sn, file, content); err != nil {
		return "", err
	}
	s.ledger.Supersede(draftFileKey(ref, sn, file), util.RevisionOf(content), anchors)
	return body, nil
}

func (s *Store) Edit(
	ctx context.Context,
	session, name, file, hash string,
	edits []writetool.FlatEdit,
) (string, error) {
	if err := validFile(file); err != nil {
		return "", err
	}
	ref, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return "", err
	}
	content, ok := sn.files[file]
	if !ok {
		return "", errors.New("create the draft file first")
	}
	if len(edits) == 0 || len(edits) > 250 {
		return "", errors.New("provide between 1 and 250 anchored edits")
	}
	refs := make([]editledger.Ref, 0, 2*len(edits))
	for _, edit := range edits {
		for _, endpoint := range []string{edit.From, edit.To} {
			line, code, ok := strings.Cut(endpoint, "#")
			n, err := strconv.Atoi(line)
			if !ok || err != nil {
				return "", errors.New("copy exact LINE#HASH anchors from the editable read")
			}
			refs = append(refs, editledger.Ref{Line: n, Hash: code})
		}
	}
	key := draftFileKey(ref, sn, file)
	claim, res := s.ledger.Claim(key, hash, refs)
	if res.Outcome.Refused() {
		return "", fmt.Errorf("edit refused (%s); read the current draft range", res.Outcome.Code())
	}
	committed := false
	defer func() {
		if !committed {
			s.ledger.Release(claim)
		}
	}()
	if res.Revision != util.RevisionOf(content) {
		return "", errors.New("draft changed; read current anchors")
	}
	normalized := slices.Clone(edits)
	for i, span := range res.Lines {
		normalized[i].From = fmt.Sprintf("%d#%s", span.From, refs[2*i].Hash)
		normalized[i].To = fmt.Sprintf("%d#%s", span.To, refs[2*i+1].Hash)
	}
	next, _, spans, err := writetool.ApplyHashlineEdit(
		ctx,
		content,
		writetool.EditInput{Path: file, Hash: hash, Edits: normalized},
	)
	if err != nil {
		return "", err
	}
	if err := validText(next); err != nil {
		return "", err
	}
	start := 1
	if len(spans) > 0 {
		start = min(max(1, spans[0].From), strings.Count(next, "\n")+1)
	}
	body, anchors, err := rendered(file, next, start, 0)
	if err != nil {
		return "", err
	}
	if err := s.save(ctx, ref, sn, file, next); err != nil {
		return "", err
	}
	s.ledger.Commit(claim, util.RevisionOf(next), anchors)
	committed = true
	return body, nil
}

// Diff compares only source, tests and package metadata.
func (s *Store) Diff(ctx context.Context, session, name string) (string, error) {
	_, diff, err := s.DraftDiff(ctx, session, name)
	return diff, err
}

// DraftDiff returns the exact revision whose changes were rendered.
func (s *Store) DraftDiff(ctx context.Context, session, name string) (Package, string, error) {
	_, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return Package{}, "", err
	}
	base := sn.id.Base
	if base == "" {
		base, err = s.object(ctx, "tree", "")
		if err != nil {
			return Package{}, "", err
		}
	}
	diff, err := s.git(
		ctx,
		"",
		append([]string{"diff", "--no-ext-diff", "--no-textconv", base, sn.digest, "--"}, editableFiles...)...)
	return sn.pkg(), diff, err
}

// Rebase merges concurrent source changes onto the current release. Conflicts
// remain in the caller's draft with Git's ordinary markers for hashline editing.
func (s *Store) Rebase(ctx context.Context, session, name string) (Package, error) {
	ref, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return Package{}, err
	}
	head, err := s.ref(ctx, branch(name))
	if err != nil {
		return Package{}, err
	}
	if head == sn.id.Base {
		return sn.pkg(), nil
	}
	upstream, err := s.readSnapshot(ctx, head)
	if err != nil {
		return Package{}, err
	}
	base := &snapshot{files: map[string]string{}}
	if sn.id.Base != "" {
		base, err = s.readSnapshot(ctx, sn.id.Base)
		if err != nil {
			return Package{}, err
		}
	}
	changed := &snapshot{
		id:    identity{Name: name, SDK: sdkVersion, Base: head, Generation: rand.Text()},
		files: maps.Clone(sn.files),
	}
	for _, file := range editableFiles {
		a, aok := base.files[file]
		b, bok := sn.files[file]
		c, cok := upstream.files[file]
		switch {
		case a == c && aok == cok, b == c && bok == cok:
		case a == b && aok == bok:
			if cok {
				changed.files[file] = c
			} else {
				delete(changed.files, file)
			}
		default:
			merged, err := merge(ctx, a, b, c)
			if err != nil {
				return Package{}, err
			}
			changed.files[file] = merged
		}
	}
	changed.digest, err = s.commit(ctx, changed, "rebase "+name, sn.digest, head)
	if err != nil {
		return Package{}, err
	}
	if err := s.move(ctx, ref, changed.digest, sn.digest); err != nil {
		return Package{}, err
	}
	return changed.pkg(), nil
}

func merge(ctx context.Context, base, ours, theirs string) (string, error) {
	dir, err := os.MkdirTemp("", "cozyphi-merge-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	for name, data := range map[string]string{"base": base, "ours": ours, "theirs": theirs} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			return "", err
		}
	}
	// --stdout returns the marked text even when merge-file signals conflicts.
	cmd := exec.CommandContext(ctx, "git", "merge-file", "--stdout", "--", "ours", "base", "theirs")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var status *exec.ExitError
		if !errors.As(err, &status) || status.ExitCode() < 1 || status.ExitCode() > 127 {
			return "", fmt.Errorf("merge function changes: %w", err)
		}
	}
	return string(out), nil
}
