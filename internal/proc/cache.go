package proc

const (
	CacheLines = 8
	wordSize   = 4
)

type cacheLine struct {
	valid bool
	index int
	tag   int
	value int32
}

type Cache struct {
	Enabled bool
	lines   []cacheLine
	Hits    int
	Misses  int
}

func NewCache(n int) *Cache {
	c := &Cache{Enabled: true, lines: make([]cacheLine, n)}
	for i := range c.lines {
		c.lines[i].index = i
	}
	return c
}

func (c *Cache) split(addr int) (index, tag int) {
	block := addr / wordSize
	return block % len(c.lines), block / len(c.lines)
}

func (c *Cache) addrOf(l cacheLine) int {
	return (l.tag*len(c.lines) + l.index) * wordSize
}

func (c *Cache) lookup(addr int) (int32, bool) {
	i, tag := c.split(addr)
	if l := c.lines[i]; l.valid && l.tag == tag {
		c.Hits++
		return l.value, true
	}
	c.Misses++
	return 0, false
}

func (c *Cache) fill(addr int, v int32) (int, cacheLine) {
	if !c.Enabled {
		return -1, cacheLine{}
	}
	i, tag := c.split(addr)
	old := c.lines[i]
	c.lines[i] = cacheLine{valid: true, index: i, tag: tag, value: v}
	return i, old
}

func (c *Cache) update(addr int, v int32) {
	i, tag := c.split(addr)
	if l := &c.lines[i]; l.valid && l.tag == tag {
		l.value = v
	}
}
