package funclib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type lock struct {
	Packages map[string]Pin `json:"packages"`
}

func sortedKeys[T any](m map[string]T) []string { return slices.Sorted(maps.Keys(m)) }

func readLock(r io.Reader) (map[string]Pin, error) {
	raw, err := io.ReadAll(io.LimitReader(r, fileLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > fileLimit {
		return nil, errors.New("package lock is too large")
	}
	var data lock
	if err := json.Unmarshal(raw, &data); err != nil || data.Packages == nil {
		return nil, errors.New("invalid function lock; provide a packages object with exact versions and digests")
	}
	if len(data.Packages) > maxPackages {
		return nil, errors.New("too many packages in lock")
	}
	for n, p := range data.Packages {
		if !nameOK(n) || !versionOK(p.Version) || !validOID.MatchString(p.Digest) {
			return nil, errors.New("invalid package name, version or digest in lock")
		}
	}
	return data.Packages, nil
}

func projectRoot(project string) (string, error) {
	if project == "" {
		return "", errors.New("checkout root is required for package resolution")
	}
	abs, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func portablePins(root string) (map[string]Pin, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	const file = "cozyphi.functions.lock"
	info, err := dir.Lstat(file)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Pin{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("cozyphi.functions.lock must be a regular file")
	}
	f, err := dir.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLock(f)
}

type projectLock struct {
	ref, oid        string
	local, portable map[string]Pin
}

func (s *Store) project(ctx context.Context, project string) (projectLock, error) {
	root, err := projectRoot(project)
	if err != nil {
		return projectLock{}, err
	}
	l := projectLock{ref: "refs/cozy/locks/" + opaque(root), local: make(map[string]Pin)}
	l.portable, err = portablePins(root)
	if err != nil {
		return l, err
	}
	exists, err := s.exists()
	if !exists || err != nil {
		return l, err
	}
	l.oid, err = s.ref(ctx, l.ref)
	if err != nil || l.oid == "" {
		return l, err
	}
	raw, err := s.git(ctx, "", "cat-file", "blob", l.oid)
	if err != nil {
		return l, err
	}
	l.local, err = readLock(strings.NewReader(raw))
	return l, err
}

func (l projectLock) roots() map[string]Pin {
	roots := maps.Clone(l.local)
	maps.Copy(roots, l.portable)
	return roots
}

// graph verifies a pin against its release tag before loading its source.
// Stored dependency graphs already have full transitive pins, so each package
// is visited at most once across the roots.
func (s *Store) graph(ctx context.Context, roots map[string]Pin) (map[string]Pin, error) {
	graph := maps.Clone(roots)
	if graph == nil {
		graph = make(map[string]Pin)
	}
	queue := sortedKeys(graph)
	for i := 0; i < len(queue); i++ {
		if len(queue) > maxPackages {
			return nil, errors.New("function dependency graph is too large")
		}
		name := queue[i]
		pin := graph[name]
		meta, err := s.releaseIdentity(ctx, pin.Digest)
		if err != nil {
			return nil, err
		}
		if meta.Name != name || meta.Version != pin.Version {
			return nil, errors.New("stored release has a mismatched identity")
		}
		for _, dep := range sortedKeys(meta.Pins) {
			p := meta.Pins[dep]
			if prior, ok := graph[dep]; ok {
				if prior != p {
					return nil, fmt.Errorf(
						"incompatible pinned versions of %s; upgrade the dependent roots together",
						dep,
					)
				}
				continue
			}
			graph[dep] = p
			queue = append(queue, dep)
		}
	}
	if len(graph) > 0 {
		var request strings.Builder
		names := sortedKeys(graph)
		for _, name := range names {
			fmt.Fprintln(&request, tag(name, graph[name].Version))
		}
		out, err := s.git(ctx, request.String(), "cat-file", "--batch-check=%(objectname) %(objecttype)")
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		if len(names) != len(lines) {
			return nil, errors.New("failed to verify the complete set of pinned revisions")
		}
		for i, name := range names {
			if lines[i] != graph[name].Digest+" commit" {
				return nil, fmt.Errorf("missing or mismatched immutable release %s@%s", name, graph[name].Version)
			}
		}
	}
	return graph, nil
}

func (s *Store) sources(ctx context.Context, pins map[string]Pin) (map[string]string, error) {
	mods := make(map[string]string, len(pins))
	total := 0
	for name, pin := range pins {
		sn, err := s.readSnapshot(ctx, pin.Digest)
		if err != nil {
			return nil, err
		}
		src := sn.files["index.ts"]
		total += len(src)
		if total > 8<<20 {
			return nil, errors.New("loaded library modules exceed 8 MiB")
		}
		mods[name+"/index.ts"] = src
	}
	return mods, nil
}

// Catalog overlays the project's pins on the latest package heads. Reads do not create locks.
func (s *Store) Catalog(ctx context.Context, project string) ([]Package, error) {
	l, err := s.project(ctx, project)
	if err != nil {
		return nil, err
	}
	graph, err := s.graph(ctx, l.roots())
	if err != nil {
		return nil, err
	}
	return s.list(ctx, "refs/heads/packages", graph)
}

// Pin explicitly selects a release for this checkout. Portable locks are
// user-owned project files edited through regular workspace permissions.
func (s *Store) Pin(ctx context.Context, project, name, version string) (Package, error) {
	if !versionOK(version) {
		return Package{}, errors.New("pin requires an exact version")
	}
	pkg, err := s.Get(ctx, name, version)
	if err != nil {
		return Package{}, err
	}
	_, _, err = s.resolve(
		ctx,
		project,
		[]string{name},
		map[string]Pin{name: {Version: version, Digest: pkg.Digest}},
		true,
	)
	return pkg, err
}

// PinSet upgrades related root packages in one project-lock transaction.
func (s *Store) PinSet(ctx context.Context, project string, versions map[string]string) (map[string]Pin, error) {
	if len(versions) == 0 || len(versions) > 128 {
		return nil, errors.New("select between 1 and 128 exact package versions")
	}
	pins := make(map[string]Pin, len(versions))
	for _, name := range sortedKeys(versions) {
		version := versions[name]
		if !versionOK(version) {
			return nil, errors.New("each pin requires an exact semantic version")
		}
		digest, err := s.getOID(ctx, name, version)
		if err != nil {
			return nil, err
		}
		pins[name] = Pin{Version: version, Digest: digest}
	}
	_, resolved, err := s.resolve(ctx, project, sortedKeys(pins), pins, true)
	return resolved, err
}

func (s *Store) Resolve(ctx context.Context, project string, names []string) (map[string]string, error) {
	m, _, err := s.ResolvePinned(ctx, project, names)
	return m, err
}

func (s *Store) Preview(ctx context.Context, project string, names []string) (map[string]string, error) {
	m, _, err := s.PreviewPinned(ctx, project, names)
	return m, err
}

func (s *Store) ResolvePinned(
	ctx context.Context,
	project string,
	names []string,
) (map[string]string, map[string]Pin, error) {
	return s.resolve(ctx, project, names, nil, true)
}

func (s *Store) PreviewPinned(
	ctx context.Context,
	project string,
	names []string,
) (map[string]string, map[string]Pin, error) {
	return s.resolve(ctx, project, names, nil, false)
}

func (s *Store) resolve(
	ctx context.Context,
	project string,
	names []string,
	overrides map[string]Pin,
	persist bool,
) (map[string]string, map[string]Pin, error) {
	if len(names) == 0 {
		return map[string]string{}, map[string]Pin{}, nil
	}
	if len(names) > 128 {
		return nil, nil, errors.New("a program may import up to 128 libraries")
	}
	for _, name := range names {
		if !nameOK(name) {
			return nil, nil, errors.New("invalid imported package name")
		}
	}
	var conflict error
	for range 6 {
		l, err := s.project(ctx, project)
		if err != nil {
			return nil, nil, err
		}
		initial := maps.Clone(l.local)
		for name, override := range overrides {
			if fixed, ok := l.portable[name]; ok && fixed != override {
				return nil, nil, fmt.Errorf(
					"update %s in the project's cozyphi.functions.lock to select another version",
					name,
				)
			}
			l.local[name] = override
		}
		pinned, err := s.graph(ctx, l.roots())
		if err != nil {
			return nil, nil, err
		}
		needed, err := s.importRoots(ctx, names, pinned)
		if err != nil {
			return nil, nil, err
		}
		for name, chosen := range needed {
			if _, portable := l.portable[name]; !portable {
				l.local[name] = chosen
			}
		}
		chosen, err := s.graph(ctx, needed)
		if err != nil {
			return nil, nil, err
		}
		for name, p := range chosen {
			if v, ok := pinned[name]; ok && p != v {
				return nil, nil, fmt.Errorf("package %s conflicts with another project pin", name)
			}
		}
		modules, err := s.sources(ctx, chosen)
		if err != nil {
			return nil, nil, err
		}
		if !persist || maps.Equal(initial, l.local) {
			return modules, chosen, nil
		}
		raw, err := json.Marshal(lock{Packages: l.local})
		if err != nil {
			return nil, nil, err
		}
		oid, err := s.object(ctx, "blob", string(raw))
		if err != nil {
			return nil, nil, err
		}
		if conflict = s.move(ctx, l.ref, oid, l.oid); conflict == nil {
			return modules, chosen, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
	}
	return nil, nil, fmt.Errorf("project lock changed repeatedly; retry resolution: %w", conflict)
}

// A fresh program may import both a package and one of its dependencies.
// Prefer the dependency its caller actually tested over the newest release.
func (s *Store) importRoots(ctx context.Context, names []string, pinned map[string]Pin) (map[string]Pin, error) {
	selected := make(map[string]Pin, len(names))
	for _, name := range names {
		p, ok := pinned[name]
		if !ok {
			digest, err := s.getOID(ctx, name, "")
			if err != nil {
				return nil, err
			}
			id, err := s.releaseIdentity(ctx, digest)
			if err != nil {
				return nil, err
			}
			p = Pin{Version: id.Version, Digest: digest}
		}
		selected[name] = p
	}
	if len(selected) < 2 {
		return selected, nil
	}
	for range len(selected) + 1 {
		constraints := make(map[string]Pin)
		for _, parent := range sortedKeys(selected) {
			meta, err := s.releaseIdentity(ctx, selected[parent].Digest)
			if err != nil {
				return nil, err
			}
			for dep, required := range meta.Pins {
				if _, imported := selected[dep]; !imported {
					continue
				}
				if prior, ok := constraints[dep]; ok && prior != required {
					return nil, fmt.Errorf("imports require incompatible versions of %s; pin compatible roots", dep)
				}
				constraints[dep] = required
			}
		}
		changed := false
		for name, p := range constraints {
			if existing, fixed := pinned[name]; fixed && existing != p {
				return nil, fmt.Errorf("a pinned release of %s conflicts with the imported package", name)
			}
			if selected[name] != p {
				selected[name] = p
				changed = true
			}
		}
		if !changed {
			return selected, nil
		}
	}
	return nil, errors.New("unable to select consistent import versions; pin compatible roots")
}

// Inventory lists the revisions visible to this session without opening their
// source files. The selector hydrates only revisions absent from its index.
func (s *Store) Inventory(ctx context.Context, project, session string) ([]Package, []Package, error) {
	l, err := s.project(ctx, project)
	if err != nil {
		return nil, nil, err
	}
	pins, err := s.graph(ctx, l.roots())
	if err != nil {
		return nil, nil, err
	}
	releases, err := s.heads(ctx, "refs/heads/packages")
	if err != nil {
		return nil, nil, err
	}
	for name, pin := range pins {
		releases[name] = pin.Digest
	}
	var drafts map[string]string
	if session != "" {
		drafts, err = s.heads(ctx, "refs/heads/drafts/"+opaque(session))
		if err != nil {
			return nil, nil, err
		}
	}
	return inventory(releases), inventory(drafts), nil
}

func inventory(heads map[string]string) []Package {
	items := make([]Package, 0, len(heads))
	for _, name := range sortedKeys(heads) {
		items = append(items, Package{Name: name, Digest: heads[name]})
	}
	return items
}

// Load retrieves an immutable revision selected by Inventory. Callers exposing
// packages to models must retain Inventory's checkout and session scoping.
func (s *Store) Load(ctx context.Context, digest string) (Package, error) {
	sn, err := s.readSnapshot(ctx, digest)
	if err != nil {
		return Package{}, err
	}
	return sn.pkg(), nil
}
