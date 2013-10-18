package events

type ProxyEvent struct {
  Name string // ie. "hit", "miss"
  Host string // ie. "localhost", "romano.com"
}

type ProxyEvents chan *ProxyEvent

func (c ProxyEvents) Create(name, host string) {
  c <- &ProxyEvent{name, host}
}