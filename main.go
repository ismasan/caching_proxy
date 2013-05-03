package main

import (
    "log"
    "fmt"
    "net/http"
    "strings"
    // "net/http/httputil"
    // "strconv"
    // "github.com/gorilla/mux"
    // "io/ioutil"
    // "net/url"
    // "bootic_pageviews/udp"
    // "bootic_pageviews/request"
    "flag"
)

type Proxy struct {
  backends []string
  redis_host string
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  fmt.Fprintf(w, "Hello, %q", r.URL.Path)
}

func NewProxy(backend_hosts, redis_host string) (proxy *Proxy, err error) {
  split_backends := strings.Split(backend_hosts, ",")
  
  proxy = &Proxy{backends: split_backends, redis_host: redis_host}
  return
}

func main() {
  /* Parse CLI flags
  ------------------------------*/
  var(
    listen_host string
    backend_hosts string
    redis_host string
    api_host string
  )
  
  flag.StringVar(&listen_host, "listen", "localhost:3000", "HTTP host:port to listen for incoming requests")
  flag.StringVar(&backend_hosts, "memcached", "", "Comma-separated list of host:port backend servers to proxy traffic to")
  flag.StringVar(&redis_host, "redis", "localhost:6379", "host:port Redis host to cache requests in")
  flag.StringVar(&api_host, "api", "localhost:7000", "HTTP host:port to server API from")
  
  flag.Parse()
  
  var proxy, _ = NewProxy(backend_hosts, redis_host)
  
  http.Handle("/", proxy)
  
  log.Println("Proxying HTTP requests on", listen_host)
  log.Println("Proxying requests to backends", backend_hosts)
  log.Println("Caching data requests in Redis", redis_host)
  log.Println("Serving API on", api_host)
  
  log.Fatal(http.ListenAndServe(listen_host, nil))
  
}