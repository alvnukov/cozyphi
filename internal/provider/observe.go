package provider

import (
	"slices"
	"strconv"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// Observation reports the catalog and credential-store state the read-only
// harness view may publish. It is a projection rather than an accessor for
// the same reason the model's is: Info carries endpoints, AuthMethod carries
// the endpoint a credential is pinned to, and credential carries the key, the
// tokens, the account id and the expiry. None of those types crosses this
// function, so what the harness can say about a credential is exactly what is
// written below — that one exists, and for which provider.
//
// It reads what is already in memory under one read lock: no catalog refresh,
// no re-read of either file, and no authentication. A nil manager is the "no
// provider manager" case, not an error, and every layer fed from it reports
// unavailable.
func (m *Manager) Observation() diag.ProviderFacts {
	if m == nil {
		return diag.ProviderFacts{}
	}
	m.mu.RLock()
	catalog := len(m.providers)
	cached := m.catalogCached
	revision := m.storeRevision
	connected := make([]string, 0, len(m.credentials))
	for id := range m.credentials {
		connected = append(connected, id)
	}
	m.mu.RUnlock()
	// Provider ids are validated on the way in — both the cache and the
	// credential file reject an id that is not one — so the names are safe to
	// publish. Sorting makes two observations of one store comparable.
	slices.Sort(connected)
	return diag.ProviderFacts{
		Known:     true,
		Catalog:   catalog,
		Cached:    cached,
		Connected: connected,
		Revision:  strconv.FormatUint(revision, 10),
	}
}
