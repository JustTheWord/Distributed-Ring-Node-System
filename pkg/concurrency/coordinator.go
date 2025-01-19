package concurrency

import "sync"

// Coordinator is a simple centralized mutual-exclusion manager.
type Coordinator struct {
	mu            sync.Mutex
	isLocked      bool
	currentHolder string
	requestQueue  []string
}

func NewCoordinator() *Coordinator {
	return &Coordinator{
		requestQueue: make([]string, 0),
	}
}

// RequestAccess either grants immediately if free or queues the request.
func (c *Coordinator) RequestAccess(nodeID string) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isLocked {
		c.isLocked = true
		c.currentHolder = nodeID
		return true, "Granted"
	}
	// otherwise queue the request
	c.requestQueue = append(c.requestQueue, nodeID)
	return false, "Queued"
}

// ReleaseAccess releases if the caller is the current holder, then grants to next in queue.
func (c *Coordinator) ReleaseAccess(nodeID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if nodeID == c.currentHolder {
		c.isLocked = false
		c.currentHolder = ""
		if len(c.requestQueue) > 0 {
			next := c.requestQueue[0]
			c.requestQueue = c.requestQueue[1:]
			c.isLocked = true
			c.currentHolder = next
		}
		return true
	}
	return false
}
