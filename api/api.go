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
  Hits int `json:"hits"`
  Misses int `json:"misses"`
  Purges int `json:"purges"`
}

type Api struct {
  router *mux.Router
  Stats *Stats
  CacheController CacheController
}

func NewApi(controller CacheController) (api *Api, err error) {
  api = &Api{
    Stats: &Stats{UpSince: time.Now()},
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
      // host  := event.Host
      if name == "hit" {
        a.Stats.Hits = a.Stats.Hits + 1
      } else if name == "miss" {
        a.Stats.Misses = a.Stats.Misses + 1
      } else if name == "purge" {
        a.Stats.Purges = a.Stats.Purges + 1
      }
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