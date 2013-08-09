package main

import (
	"caching_proxy/proxy"
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
		redis_host    string
		api_host      string
	)

	flag.StringVar(&listen_host, "listen", "localhost:3000", "HTTP host:port to listen for incoming requests")
	flag.StringVar(&backend_hosts, "backends", "", "Comma-separated list of host:port backend servers to proxy traffic to")
	flag.StringVar(&redis_host, "redis", "localhost:6379", "host:port Redis host to cache requests in")
	flag.StringVar(&api_host, "api", "localhost:7000", "HTTP host:port to serve API from")

	flag.Parse()

	var proxy, _ = proxy.NewProxy(backend_hosts, redis_host)

	http.Handle("/", proxy)

	log.Println("Proxying HTTP requests on", listen_host)
	log.Println("Proxying requests to backends", backend_hosts)
	log.Println("Caching data requests in Redis", redis_host)
	log.Println("Serving API on", api_host)

	log.Fatal(http.ListenAndServe(listen_host, nil))

}
