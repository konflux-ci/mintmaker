package base

import (
	"sync"
)

type Cache struct {
	data sync.Map
}

func NewCache() *Cache {
	return &Cache{}
}

func (c *Cache) Get(key string) (interface{}, bool) {
	return c.data.Load(key)
}

func (c *Cache) Set(key string, value interface{}) {
	c.data.Store(key, value)
}
