package proxy

import (
    "log"
    "encoding/json"
    // "io"
    "net"
    "net/url"
    "net/http"
    "strings"
    "io"
    "io/ioutil"
    "github.com/bradfitz/gomemcache/memcache"
)

type Store interface {

  Get(key string) ([]byte, error)
  Set(key string, data []byte) (error)

}

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

type Proxy struct {
  backends []*url.URL
  Store Store
  // The transport used to perform proxy requests.
  // If nil, http.DefaultTransport is used.
  // http.RoundTripper is an interface
  Transport http.RoundTripper
}

func NewProxy(backend_hosts, store_hosts string) (proxy *Proxy, err error) {
  split_backends := strings.Split(backend_hosts, ",")
  var backend_urls []*url.URL
  for _, host := range split_backends {
    url, err := url.Parse(host)
    if err != nil {
      log.Fatal(err)
    }
    backend_urls = append(backend_urls, url)
  }

  store := NewMemcacheStore(store_hosts)

  proxy = &Proxy{backends: backend_urls, Store: store}
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

  response = &CachedResponse{
    Body: body, 
    Headers: res.Header, 
    Status: res.Status,
    ContentLength: res.ContentLength,
  }
  
  return
}

func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
  
  /* Is it cacheable?
  ------------------------------------*/
  if req.Method == "GET" || req.Method == "HEAD" || req.Method == "OPTIONS" { // can be cache
    /* Is it cached?
    ------------------------------------*/
    cacheKey := p.cacheKey(req)

    data, err := p.Store.Get(cacheKey)
    if err == nil { // cache hit. Serve it.
      log.Println("CACHE HIT", cacheKey)
      p.serveFromCache(data, rw)
    } else { // Cache miss. Proxy and cache.
      log.Println("CACHE MISS", cacheKey, err, data)
      p.proxyAndCache(cacheKey, rw, req)
    }

  } else { // just proxy to backends
    log.Println("NON CACHEABLE")
    backendResp, err := p.proxy(rw, req)
    if err != nil {
      log.Printf("http: proxy error: %v", err)
      rw.WriteHeader(http.StatusInternalServerError)
      return
    }
    copyHeaderForFrontend(rw.Header(), backendResp.Header)
    io.Copy(rw, backendResp.Body)
  }
}

func (p *Proxy) director(req *http.Request, backend *url.URL) {
  targetQuery := backend.RawQuery
  
  req.URL.Scheme = backend.Scheme
  req.URL.Host = backend.Host
  req.URL.Path = singleJoiningSlash(backend.Path, req.URL.Path)
  if targetQuery == "" || req.URL.RawQuery == "" {
    req.URL.RawQuery = targetQuery + req.URL.RawQuery
  } else {
    req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
  }
}

func (p *Proxy) cacheKey(req *http.Request) string {
  key := req.URL.String()
  
  s := []string{"caching", req.Method, req.Host, key}
  return strings.Join(s, ":")
}

func (p *Proxy) proxy(rw http.ResponseWriter, req *http.Request) (*http.Response, error) {
  backend := p.backends[0] // do round-robin here later

  /* Add forward
  -----------------------------*/
  transport := p.Transport
  if transport == nil {
    transport = http.DefaultTransport
  }

  outreq := new(http.Request)
  *outreq = *req // includes shallow copies of maps, but okay

  p.director(outreq, backend)
  outreq.Method = req.Method
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
    copyHeaderForBackend(outreq.Header, req.Header)
    outreq.Header.Del("Connection")
  }

  if clientIp, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
    outreq.Header.Set("X-Forwarded-For", clientIp)
  }

  return transport.RoundTrip(outreq)
}

func (p *Proxy) proxyAndCache(cacheKey string, rw http.ResponseWriter, req *http.Request) {

  backendResp, err := p.proxy(rw, req)

  if err != nil {
    log.Printf("http: proxy error: %v", err)
    rw.WriteHeader(http.StatusInternalServerError)
    return
  }

  log.Println("http: response code: ", backendResp.StatusCode)
  /* Cache in Redis
  ------------------------------------*/
  cached_response := NewCachedResponse(backendResp)
  /* Copy headers
  ------------------------------------*/
  copyHeaderForFrontend(rw.Header(), backendResp.Header)
  
  /* Only cache successful response
  ------------------------------------*/
  var body []byte
  body = cached_response.Body

  if backendResp.StatusCode >= 200 && backendResp.StatusCode < 300 {
    log.Println("SUCCESS")
    if req.Method == "HEAD" || req.Method == "OPTIONS" {
      log.Println("NO BODY", req.Method)
      rw.Write([]byte{})
      req.Body.Close()
    } else if req.Header.Get("If-Modified-Since") != "" && req.Header.Get("If-None-Match") != "" {
      // if client is sending if-modified or if-non-match
      // we assume that they already have a copy of the body
      log.Println("Client has copy. Send 304")
      rw.WriteHeader(http.StatusNotModified)
      body = []byte{}
    } else {
      /* Copy status code
      ------------------------------------*/
      rw.WriteHeader(backendResp.StatusCode)
    }

    rw.Write(body)
    p.cache(cacheKey, cached_response)
  } else { // error. Copy body.
    log.Println("Error Status", backendResp.StatusCode)
    rw.WriteHeader(backendResp.StatusCode)
    io.Copy(rw, backendResp.Body)
  }

}

func (p *Proxy) serveFromCache(data []byte, rw http.ResponseWriter) {
  cached_response, _ := deserializeResponse(data)
  /* Copy headers
  ------------------------------------*/
  copyHeaderForFrontend(rw.Header(), cached_response.Headers)

  rw.Header().Add("X-Cache", "GoProxy")
  rw.Write(cached_response.Body)
}

func (p *Proxy) cache(key string, cached_response *CachedResponse) {
  // encode
  log.Println("CACHE NOW", cached_response.Headers)
  encoded, _ := serializeResponse(cached_response)
  p.Store.Set(key, encoded)
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

func copyHeaderForFrontend(dst, src http.Header) {
  for k, vv := range src {
    for _, v := range vv {
      dst.Add(k, v)
    }
  }
}
// We don not want the backend to respond with 304
// Because then we don't have a response body to cache!
func copyHeaderForBackend(dst, src http.Header) {
  for k, vv := range src {
    for _, v := range vv {
      if k != "If-Modified-Since" && k != "If-None-Match" {
        dst.Add(k, v)
      }
    }
  }
}