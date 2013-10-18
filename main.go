package main

import (
  "caching_proxy/proxy"
  "caching_proxy/api"
  "flag"
  "log"
  "net/http"
)

func main() {
  /* Parse CLI flags
  ------------------------------*/
  var (
    listen_host   string
    backend_hosts string
    memcache_host    string
    api_host      string
    udp_host  string
  )
  
  flag.StringVar(&listen_host, "listen", "localhost:3000", "HTTP host:port to listen for incoming requests")
  flag.StringVar(&backend_hosts, "backends", "", "Comma-separated list of host:port backend servers to proxy traffic to")
  flag.StringVar(&memcache_host, "memcache", "localhost:11211", "comma-separated host:port memcache host to cache requests in")
  flag.StringVar(&api_host, "api", "localhost:7000", "HTTP host:port to serve API from")
  flag.StringVar(&udp_host, "udphost", "localhost:5555", "UDP host:port to send packets to")

  flag.Parse()

  /* Caching proxy
  ------------------------------------*/
  proxy, err := proxy.NewProxy(backend_hosts, memcache_host)

  if err != nil {
    log.Fatal("Error initializing proxy", err)
  }


  /* Control / stats API
  -------------------------------------*/
  api, err := api.NewApi(proxy)

  if err != nil {
    log.Fatal("Error initializing API", err)
  }
  
  // API keeps in-memory counts
  api.SubscribeTo(proxy.Events)

  log.Println("Proxying HTTP requests on", listen_host)
  log.Println("Proxying requests to backends", backend_hosts)
  log.Println("Caching data requests in Memcache", memcache_host)
  log.Println("Serving API on", api_host)

  go http.ListenAndServe(api_host, api)
  
  log.Fatal(http.ListenAndServe(listen_host, proxy))

}
