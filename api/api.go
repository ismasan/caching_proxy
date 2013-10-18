package api

import (
  "net/http"
  "log"
  "github.com/gorilla/mux"
  "caching_proxy/store"
)

type Api struct {
  store store.Store
  router *mux.Router
}

func (a *Api) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
  log.Println(a.store)
  a.router.ServeHTTP(rw, req)
}

func (a *Api) handleRoot(rw http.ResponseWriter, req *http.Request) {
  rw.Write([]byte("API root"))
}

func NewApi(st store.Store) (api *Api, err error) {
  api = &Api{store: st}
  router := mux.NewRouter()
  router.HandleFunc("/", api.handleRoot).Methods("GET")
  api.router = router
  
  return
}