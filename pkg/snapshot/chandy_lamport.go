package snapshot

import "sync"

// ChandyLamportManager tracks snapshot progress: who has recorded, aggregator states, etc.
type ChandyLamportManager struct {
	mu sync.Mutex

	recorded   map[string]bool // snapshotID -> whether local state is recorded
	aggregator map[string][]string
	expected   map[string]int // snapshotID -> how many states expected
}

func NewChandyLamportManager(_ int) *ChandyLamportManager {
	return &ChandyLamportManager{
		recorded:   make(map[string]bool),
		aggregator: make(map[string][]string),
		expected:   make(map[string]int),
	}
}

// HasRecorded returns whether local state is recorded for the given snapshot ID.
func (c *ChandyLamportManager) HasRecorded(snapshotID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recorded[snapshotID]
}

// MarkRecorded flags that we've recorded the local state for a snapshot.
func (c *ChandyLamportManager) MarkRecorded(snapshotID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recorded[snapshotID] = true
}

// SetExpectedResponses sets how many node states we expect for the snapshot.
func (c *ChandyLamportManager) SetExpectedResponses(snapshotID string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expected[snapshotID] = count
}

// AddRecordedState appends a node's recorded state to aggregator storage, returns how many so far.
func (c *ChandyLamportManager) AddRecordedState(snapshotID, state string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aggregator[snapshotID] = append(c.aggregator[snapshotID], state)
	return len(c.aggregator[snapshotID])
}

// IsComplete checks if we have collected all expected states for a snapshot.
func (c *ChandyLamportManager) IsComplete(snapshotID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expected[snapshotID] == 0 {
		// If never set, we do not consider it complete
		return false
	}
	return len(c.aggregator[snapshotID]) >= c.expected[snapshotID]
}

// GetAllStates returns all recorded states for a snapshot.
func (c *ChandyLamportManager) GetAllStates(snapshotID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.aggregator[snapshotID]
}
