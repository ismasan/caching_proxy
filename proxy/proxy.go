package proxy

import (
    "log"
    "encoding/json"
    // "io"
    "net"
    "net/url"
    "net/http"
    "strings"
    "io/ioutil"
    "github.com/vmihailenco/redis"
)

type Proxy struct {
  backends []*url.URL
  Redis *redis.Client
  // The transport used to perform proxy requests.
  // If nil, http.DefaultTransport is used.
  // http.RoundTripper is an interface
  Transport http.RoundTripper
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
  
  password := "" // no password set
  redis := redis.NewTCPClient(redis_host, password, -1)
  
  defer redis.Close()
  
  proxy = &Proxy{backends: backend_urls, Redis: redis}
  return
}

type CachedResponse struct {
  Status string
  ContentLength int64
  Headers http.Header
  Body []byte
}

func serializeResponse(res *CachedResponse) (raw []byte, err error) {
  
  raw, err = json.Marshal(res)
  
  return
}

func deserializeResponse(raw []byte) (res *CachedResponse, err error) {
  
  err = json.Unmarshal(raw, &res)
  
  return
}

func NewCachedResponse(res *http.Response) (response *CachedResponse) {
  body, err := ioutil.ReadAll(res.Body)
  if err != nil {
     log.Fatal("Error reading from Body", err)
  }
  
  // var headers map[string][]string
  //   
  //   for k, vv := range res.Header {
  //     headers[k] = vv
  //   }
  
  response = &CachedResponse{
    Body: body, 
    Headers: res.Header, 
    Status: res.Status,
    ContentLength: res.ContentLength,
  }
  
  return
}

func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
  /* Is it cached?
  ------------------------------------*/
  cacheKey := p.cacheKey(req)
  
  hit := p.Redis.Get(cacheKey)
  
  if hit.Err() == nil { // cache hit. Serve it.
    log.Println("CACHE HIT", cacheKey)
    p.serveFromCache(hit.Val(), rw)
  } else { // Cache miss. Proxy and cache.
    log.Println("CACHE MISS", cacheKey)
    p.proxyAndCache(cacheKey, rw, req)
  }
  
  /* Copy body
  -----------------------------------*/
  // Buffered version
  // rw.Write(cached_response.Body)
  // Stream version
  // if res.Body != nil {
  //     var dst io.Writer = rw
  //     io.Copy(dst, res.Body)
  //   }
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

func (p *Proxy) cacheKey(req *http.Request) string {
  key := req.URL.String()
  
  s := []string{"caching", req.Host, key}
  return strings.Join(s, ":")
}

func (p *Proxy) proxyAndCache(cacheKey string, rw http.ResponseWriter, req *http.Request) {
  target := p.backends[0] // do round-robin here later

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
  
  /* Cache in Redis
  ------------------------------------*/
  cached_response := NewCachedResponse(res)
  /* Copy headers
  ------------------------------------*/
  copyHeader(rw.Header(), res.Header)
  
  rw.WriteHeader(res.StatusCode)
  rw.Write(cached_response.Body)
  
  go p.cache(cacheKey, cached_response)
}

func (p *Proxy) serveFromCache(data string, rw http.ResponseWriter) {
  cached_response, _ := deserializeResponse([]byte(data))
  
  /* Copy headers
  ------------------------------------*/
  copyHeader(rw.Header(), cached_response.Headers)
  
  // rw.WriteHeader(cached_response.StatusCode)
  rw.Write(cached_response.Body)
}

func (p *Proxy) cache(key string, cached_response *CachedResponse) {
  // encode
  encoded, _ := serializeResponse(cached_response)
  p.Redis.Set(key, string(encoded))
}

/* Utils
--------------------------*/

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