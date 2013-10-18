package api

import (
  "net/http"
  "log"
  "time"
  "github.com/gorilla/mux"
  "caching_proxy/events"
  "encoding/json"
)

// Proxy implements this
type CacheController interface {
  PurgeHost(hostName string)
}

type Stats struct {
  UpSince time.Time `json:"up_since"`
  Totals map[string]int `json:"totals"`
  Hosts map[string]map[string]int `json:"hosts"`
}

func (s *Stats) Op(opName string) {
  s.Totals[opName]++
}

func (s *Stats) HostOp(hostName, opName string) {
  mm, ok := s.Hosts[hostName]
  if !ok {
      mm = make(map[string]int)
      s.Hosts[hostName] = mm
  }
  s.Hosts[hostName][opName]++
}

type Api struct {
  router *mux.Router
  Stats *Stats
  CacheController CacheController
}

func NewApi(controller CacheController) (api *Api, err error) {
  api = &Api{
    Stats: &Stats{
      UpSince: time.Now(),
      Totals: make(map[string]int),
      Hosts: make(map[string]map[string]int),
    },
    CacheController: controller,
  }
  router := mux.NewRouter()
  router.HandleFunc("/hosts/{hostname}", api.handlePurgeHost).Methods("PUT")
  router.HandleFunc("/", api.handleRoot).Methods("GET")
  api.router = router

  return
}

func (a *Api) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
  rw.Header().Add("Content-Type", "application/json")
  rw.Header().Add("Cache-Control", "no-store, no-cache, must-revalidate, private, proxy-revalidate")
  rw.Header().Add("Pragma", "no-cache")
  rw.Header().Add("Expires", "Fri, 24 Nov 2000 01:00:00 GMT")
  a.router.ServeHTTP(rw, req)
}

func (a *Api) SubscribeTo(ch events.ProxyEvents) {
  go func(ch events.ProxyEvents) {
    for {
      event := <- ch
      name  := event.Name
      host  := event.Host
      a.Stats.Op(name)
      a.Stats.HostOp(host, name)
    }
  }(ch)
}

/* URL handlers
----------------------------------*/
func (a *Api) handleRoot(rw http.ResponseWriter, req *http.Request) {
  json, err := json.Marshal(a.Stats)

  if err != nil {
    log.Println("Error parsing JSON")
  }
  rw.Write(json)
}

func (a *Api) handlePurgeHost(rw http.ResponseWriter, req *http.Request) {
  params := mux.Vars(req)
  a.CacheController.PurgeHost(params["hostname"])
  rw.Write([]byte(""))
}