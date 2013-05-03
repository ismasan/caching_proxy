package main

import (
    "log"
    // "fmt"
    "io"
    "net"
    "net/url"
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
  backends []*url.URL
  redis_host string
  // The transport used to perform proxy requests.
  // If nil, http.DefaultTransport is used.
  Transport http.RoundTripper
}

func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
  // fmt.Fprintf(rw, "Hello, %q", req.URL.Path)
  p.proxyRequest(rw, req)
}

func singleJoiningSlash(a, b string) string {
  aslash := strings.HasSuffix(a, "/")
  bslash := strings.HasPrefix(b, "/")
  switch {
  case aslash && bslash:
    return a + b[1:]
  case !aslash && !bslash:
    return a + "/" + b
  }
  return a + b
}

func copyHeader(dst, src http.Header) {
  for k, vv := range src {
    for _, v := range vv {
      dst.Add(k, v)
    }
  }
}

func (p *Proxy) director(req *http.Request, target *url.URL) {
  targetQuery := target.RawQuery
  
  req.URL.Scheme = target.Scheme
  req.URL.Host = target.Host
  req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
  if targetQuery == "" || req.URL.RawQuery == "" {
    req.URL.RawQuery = targetQuery + req.URL.RawQuery
  } else {
    req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
  }
}

func (p *Proxy) proxyRequest(rw http.ResponseWriter, req *http.Request) {
  
  target := p.backends[0]
  
  /* Add forward
  -----------------------------*/
  transport := p.Transport
  if transport == nil {
    transport = http.DefaultTransport
  }
  
  outreq := new(http.Request)
  *outreq = *req // includes shallow copies of maps, but okay
  
  p.director(outreq, target)
  outreq.Proto = "HTTP/1.1"
  outreq.ProtoMajor = 1
  outreq.ProtoMinor = 1
  outreq.Close = false
  
  log.Println("Proxy", outreq.URL.Host, outreq.URL.Path, outreq.URL.RawQuery)
  
  // Remove the connection header to the backend.  We want a
  // persistent connection, regardless of what the client sent
  // to us.  This is modifying the same underlying map from req
  // (shallow copied above) so we only copy it if necessary.
  if outreq.Header.Get("Connection") != "" {
    outreq.Header = make(http.Header)
    copyHeader(outreq.Header, req.Header)
    outreq.Header.Del("Connection")
  }
  
  if clientIp, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
    outreq.Header.Set("X-Forwarded-For", clientIp)
  }
  
  res, err := transport.RoundTrip(outreq)
  if err != nil {
    log.Printf("http: proxy error: %v", err)
    rw.WriteHeader(http.StatusInternalServerError)
    return
  }
  
  copyHeader(rw.Header(), res.Header)
  
  rw.WriteHeader(res.StatusCode)
  
  if res.Body != nil {
    var dst io.Writer = rw
    // if p.FlushInterval != 0 {
    //       if wf, ok := rw.(writeFlusher); ok {
    //        dst = &maxLatencyWriter{dst: wf, latency: p.FlushInterval}
    //       }
    //     }
    io.Copy(dst, res.Body)
  }
}

func NewProxy(backend_hosts, redis_host string) (proxy *Proxy, err error) {
  split_backends := strings.Split(backend_hosts, ",")
  var backend_urls []*url.URL
  for _, host := range split_backends {
    url, err := url.Parse(host)
    if err != nil {
      log.Fatal(err)
    }
    backend_urls = append(backend_urls, url)
  }
  
  proxy = &Proxy{backends: backend_urls, redis_host: redis_host}
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
  flag.StringVar(&backend_hosts, "backends", "", "Comma-separated list of host:port backend servers to proxy traffic to")
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