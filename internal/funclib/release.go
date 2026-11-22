package funclib

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
)

// Pin identifies an immutable package release.
type Pin struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

type Validator = func(context.Context, Package, map[string]string) error

func (s *Store) heads(ctx context.Context, prefix string) (map[string]string, error) {
	yes, err := s.exists()
	if err != nil || !yes {
		return map[string]string{}, err
	}
	raw, err := s.git(ctx, "", "for-each-ref", "--count=4097", "--format=%(objectname) %(refname)", prefix+"/")
	if err != nil {
		return nil, err
	}
	heads := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		oid, ref, ok := strings.Cut(line, " ")
		name := strings.TrimPrefix(ref, prefix+"/")
		if !ok || !nameOK(name) || !validOID.MatchString(oid) {
			return nil, errors.New("invalid entry in the library catalog")
		}
		heads[name] = oid
	}
	if len(heads) > maxPackages {
		return nil, errors.New("package count exceeds the catalog limit")
	}
	return heads, nil
}

func (s *Store) List(ctx context.Context) ([]Package, error) {
	return s.list(ctx, "refs/heads/packages", nil)
}

func (s *Store) Drafts(ctx context.Context, session string) ([]Package, error) {
	if session == "" {
		return nil, errors.New("session is required")
	}
	return s.list(ctx, "refs/heads/drafts/"+opaque(session), nil)
}

func (s *Store) list(ctx context.Context, prefix string, pins map[string]Pin) ([]Package, error) {
	heads, err := s.heads(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for name, pin := range pins {
		heads[name] = pin.Digest
	}
	names := sortedKeys(heads)
	packages := make([]Package, 0, len(names))
	for _, name := range names {
		sn, err := s.readSnapshot(ctx, heads[name])
		if err != nil {
			return nil, err
		}
		if sn.id.Name != name {
			return nil, errors.New("ref name does not match the stored package")
		}
		packages = append(packages, sn.pkg())
	}
	return packages, nil
}

func (s *Store) candidate(ctx context.Context, sn *snapshot) (map[string]Pin, map[string]string, error) {
	m, err := sn.manifest()
	if err != nil {
		return nil, nil, err
	}
	src, tests := sn.files["index.ts"], sn.files["index.test.ts"]
	if strings.TrimSpace(src) == "" || strings.TrimSpace(tests) == "" {
		return nil, nil, errors.New("create nonempty index.ts and index.test.ts before testing or publishing")
	}
	for _, file := range editableFiles {
		if hasConflicts(sn.files[file]) {
			return nil, nil, fmt.Errorf("resolve merge conflicts in %s", file)
		}
	}
	dependencies := make(map[string]Pin)
	for _, name := range sortedKeys(m.Dependencies) {
		oid, err := s.getOID(ctx, name, m.Dependencies[name])
		if err != nil {
			return nil, nil, err
		}
		dependencies[name] = Pin{Version: m.Dependencies[name], Digest: oid}
	}
	graph, err := s.graph(ctx, dependencies)
	if err != nil {
		return nil, nil, err
	}
	if _, cycle := graph[sn.id.Name]; cycle {
		return nil, nil, errors.New("dependency graph contains the candidate package")
	}
	modules, err := s.sources(ctx, graph)
	if err != nil {
		return nil, nil, err
	}
	modules[sn.id.Name+"/index.ts"] = src
	modules[sn.id.Name+"/index.test.ts"] = tests
	return graph, modules, nil
}

func hasConflicts(source string) bool {
	for line := range strings.SplitSeq(source, "\n") {
		if strings.HasPrefix(line, "<<<<<<< ") || strings.HasPrefix(line, ">>>>>>> ") {
			return true
		}
	}
	return false
}

func (s *Store) Check(ctx context.Context, session, name string, check Validator) error {
	if check == nil {
		return errors.New("configure a runtime validator before testing")
	}
	_, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return err
	}
	_, mods, err := s.candidate(ctx, sn)
	if err != nil {
		return err
	}
	return check(ctx, sn.pkg(), mods)
}

// Publish validates an immutable snapshot outside Git locks. The tag, package
// head and retirement of exactly that draft advance together or none do.
func (s *Store) Publish(ctx context.Context, session, name, version string, check Validator) (Package, error) {
	return s.publish(ctx, session, name, version, "", check)
}

// PublishAt binds an approval preview to the exact tested source and tests.
func (s *Store) PublishAt(
	ctx context.Context,
	session, name, version, expected string,
	check Validator,
) (Package, error) {
	if !validOID.MatchString(expected) {
		return Package{}, errors.New("provide the exact draft revision from the approval preview")
	}
	return s.publish(ctx, session, name, version, expected, check)
}

func (s *Store) publish(
	ctx context.Context,
	session, name, version, expected string,
	check Validator,
) (Package, error) {
	if check == nil {
		return Package{}, errors.New("configure the runtime validator before publishing")
	}
	if !versionOK(version) {
		return Package{}, errors.New("provide a semantic version such as 1.0.0")
	}
	ref, sn, err := s.getDraft(ctx, session, name)
	if err != nil {
		return Package{}, err
	}
	if expected != "" && expected != sn.digest {
		return Package{}, errors.New("draft changed since preview; review the current diff")
	}
	head, err := s.ref(ctx, branch(name))
	if err != nil {
		return Package{}, err
	}
	if head != sn.id.Base {
		return Package{}, errors.New("the published package changed; rebase the draft and rerun its tests")
	}
	existing, err := s.ref(ctx, tag(name, version))
	if err != nil {
		return Package{}, err
	}
	if existing != "" {
		return Package{}, errors.New("this release already exists; choose a new version")
	}
	pins, modules, err := s.candidate(ctx, sn)
	if err != nil {
		return Package{}, err
	}
	release := *sn
	release.id = identity{Name: name, SDK: sdkVersion, Version: version, Pins: pins}
	release.digest, err = s.commit(ctx, &release, "release "+name+" v"+version, sn.digest)
	if err != nil {
		return Package{}, err
	}
	if err := check(ctx, release.pkg(), maps.Clone(modules)); err != nil {
		return Package{}, fmt.Errorf("release tests failed: %w", err)
	}
	if err := s.checkConsumers(ctx, &release, modules, check); err != nil {
		return Package{}, err
	}
	var tx strings.Builder
	fmt.Fprintf(&tx, "create %s %s\n", tag(name, version), release.digest)
	if head == "" {
		fmt.Fprintf(&tx, "create %s %s\n", branch(name), release.digest)
	} else {
		fmt.Fprintf(&tx, "update %s %s %s\n", branch(name), release.digest, head)
	}
	fmt.Fprintf(&tx, "delete %s %s\n", ref, sn.digest)
	if _, err := s.git(ctx, tx.String(), "update-ref", "--stdin"); err != nil {
		return Package{}, fmt.Errorf("package, version or draft changed during validation; reopen or rebase: %w", err)
	}
	return release.pkg(), nil
}

// Existing consumers keep their pinned versions. Before shipping a compatible
// update, exercise their current contract tests with the candidate substituted.
func (s *Store) checkConsumers(
	ctx context.Context,
	release *snapshot,
	modules map[string]string,
	check Validator,
) error {
	heads, err := s.heads(ctx, "refs/heads/packages")
	if err != nil {
		return err
	}
	newMajor, _, _ := strings.Cut(release.id.Version, ".")
	for _, name := range sortedKeys(heads) {
		dep, err := s.readSnapshot(ctx, heads[name])
		if err != nil {
			return err
		}
		old, ok := dep.id.Pins[release.id.Name]
		oldMajor, _, _ := strings.Cut(old.Version, ".")
		if !ok || oldMajor != newMajor {
			continue
		}
		candidateMods, err := s.sources(ctx, dep.id.Pins)
		if err != nil {
			return err
		}
		for id, src := range modules {
			if !strings.HasSuffix(id, ".test.ts") {
				candidateMods[id] = src
			}
		}
		candidateMods[name+"/index.ts"] = dep.files["index.ts"]
		candidateMods[name+"/index.test.ts"] = dep.files["index.test.ts"]
		if err := check(ctx, dep.pkg(), candidateMods); err != nil {
			return fmt.Errorf("dependent %s contract tests failed: %w", name, err)
		}
	}
	return nil
}
