package proc

const CacheLines = 8

type cacheLine struct {
	valid bool
	addr  int
	value int32
}

type Cache struct {
	Enabled bool
	lines   []cacheLine
	next    int
	Hits    int
	Misses  int
}

func NewCache(n int) *Cache {
	return &Cache{Enabled: true, lines: make([]cacheLine, n)}
}

func (c *Cache) lookup(addr int) (int32, bool) {
	if !c.Enabled {
		return 0, false
	}
	for _, l := range c.lines {
		if l.valid && l.addr == addr {
			c.Hits++
			return l.value, true
		}
	}
	c.Misses++
	return 0, false
}

func (c *Cache) fill(addr int, v int32) (int, cacheLine) {
	if !c.Enabled {
		return -1, cacheLine{}
	}
	i := c.next
	c.next = (c.next + 1) % len(c.lines)
	old := c.lines[i]
	c.lines[i] = cacheLine{valid: true, addr: addr, value: v}
	return i, old
}

func (c *Cache) update(addr int, v int32) bool {
	if !c.Enabled {
		return false
	}
	for i := range c.lines {
		if c.lines[i].valid && c.lines[i].addr == addr {
			c.lines[i].value = v
			return true
		}
	}
	return false
}
