package proxy

import (
  "strings"
  "github.com/bradfitz/gomemcache/memcache"
)

type MemcacheStore struct {
  client *memcache.Client
}

// If item found, always return nil error
func (s *MemcacheStore) Get(key string) (data []byte, err error) {
  item, err := s.client.Get(key)
  if item == nil {
    return
  }
  err = nil
  data = item.Value
  return
}

func (s *MemcacheStore) Set(key string, data []byte) (error) {
  s.client.Set(&memcache.Item{Key: key, Value: data})
  return nil
}

func NewMemcacheStore (hosts string) (store *MemcacheStore) {
  split_mchosts := strings.Split(hosts, ",")
  mc := memcache.New(split_mchosts...)
  store = &MemcacheStore{client: mc}
  return
}