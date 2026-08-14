package session

import (
	"fmt"
	"sync"
	"testing"

	"github.com/pulseaiclub/phi/internal/llm"
)

// TestManagerConcurrentAccess guards the mutex: concurrent append/read
// must not race or corrupt the tree. Run with -race.
func TestManagerConcurrentAccess(t *testing.T) {
	m, err := NewSessionManager(t.TempDir(), WithShouldFlush(false))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range 50 {
				msg := llm.Message{
					Role:    llm.RoleUser,
					Content: fmt.Sprintf("w%d-%d", w, i),
				}
				id, err := m.Append(msg)
				if err != nil {
					t.Errorf("append: %v", err)
					return
				}
				_ = m.BuildContext()
				_ = m.GetBranch(id)
				_ = m.Len()
			}
		}(w)
	}
	wg.Wait()

	if m.Len() < 8*50 {
		t.Fatalf("expected >= %d entries, got %d", 8*50, m.Len())
	}
}
