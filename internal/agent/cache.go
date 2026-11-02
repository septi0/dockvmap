package agent

import "sync"

type containerDetails struct {
	image      string
	startedAt  int64
	finishedAt int64
	exitCode   int
}

type dockerCache struct {
	mu         sync.Mutex
	epoch      int
	watching   bool
	info       *dockerInfo
	images     map[string][]string
	refImages  map[string]string
	containers map[string]containerDetails
}

func newDockerCache() *dockerCache {
	c := &dockerCache{}
	c.reset()

	return c
}

func (c *dockerCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.epoch++
	c.info = nil
	c.images = nil
	c.refImages = make(map[string]string)
	c.containers = make(map[string]containerDetails)
}

func (c *dockerCache) setWatching(watching bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.watching = watching
}

func (c *dockerCache) isWatching() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.watching
}

func (c *dockerCache) forgetContainer(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.epoch++
	delete(c.containers, id)
}

func (c *dockerCache) forgetImages() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.epoch++
	c.images = nil
	c.refImages = make(map[string]string)
}

func (c *dockerCache) currentEpoch() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.epoch
}

func (c *dockerCache) read(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	fn()
}

func (c *dockerCache) write(epoch int, fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.epoch == epoch {
		fn()
	}
}
