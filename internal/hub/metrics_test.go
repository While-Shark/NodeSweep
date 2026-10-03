package hub

import (
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "testing/fstest"
 "time"
 "github.com/While-Shark/NodeSweep/internal/store"
 "github.com/While-Shark/NodeSweep/internal/engine"
)
func TestMetricsHistoryAuthenticationAndNodeIsolation(t *testing.T) {
 s,err:=store.Open(filepath.Join(t.TempDir(),"state.db"));if err!=nil{t.Fatal(err)};defer s.DB.Close()
 h:=&Hub{Store:s,Token:strings.Repeat("a",64)};handler:=h.Handler(fstest.MapFS{})
 s.AddNode(store.Node{ID:"one"},Hash("agent"));s.AddNode(store.Node{ID:"two"},"")
 if err=s.UpdateNode(store.Node{ID:"one",Metrics:engine.Metrics{CPU:27},LastSeen:time.Now()});err!=nil{t.Fatal(err)}
 for _,test:=range []struct{path,token string;code int;body string}{
  {"metrics/one","agent",401,""}, {"metrics/missing",h.Token,404,""},
  {"metrics/one?period=all",h.Token,400,""}, {"metrics/two",h.Token,200,"[]"},
  {"metrics/one?period=7d",h.Token,200,"\"cpu\":27"},
 } {
  req:=httptest.NewRequest("GET","/api/"+test.path,nil);req.Header.Set("Authorization","Bearer "+test.token)
  res:=httptest.NewRecorder();handler.ServeHTTP(res,req)
  if res.Code!=test.code || !strings.Contains(res.Body.String(),test.body){t.Fatal(test.path,res.Code,res.Body.String())}
 }
}
