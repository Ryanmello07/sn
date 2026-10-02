package miner

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
)

// A request for readiness must never receive the legacy process-liveness
// response when no provider observation owner exists.
func TestProviderProgressStatusRefusesUnownedReadiness(t *testing.T) {
 response:=httptest.NewRecorder()
 (&Status{}).ServeHTTP(response,httptest.NewRequest(http.MethodGet,"/provider-progress",nil))
 if response.Code!=http.StatusServiceUnavailable {t.Fatal("provider readiness request received unowned process liveness",response.Code,response.Body.String())}
}

// The actual public swarm lifetime exposes each configured member even when
// an instance has no SDK observation. Running count is not identity evidence.
func TestProviderProgressSwarmExposesMemberCensus(t *testing.T) {
 fixture:=newSwarmPublishedFixture(t)
 response:=httptest.NewRecorder()
 fixture.swarm.ServeHTTP(response,httptest.NewRequest(http.MethodGet,"/provider-progress",nil))
 var value struct {Schema string `json:"schema"`;Members []struct{Slot string `json:"slot"`;Ready bool `json:"ready"`}} 
 if err:=json.Unmarshal(response.Body.Bytes(),&value);err!=nil||response.Code!=http.StatusOK||value.Schema!="urnetwork-provider-progress-v1"||len(value.Members)!=2 {t.Fatal("public provider progress omitted actual member census",response.Code,response.Body.String(),err)}
 if value.Members[0].Ready||value.Members[1].Ready {t.Fatal("lifecycle count fabricated provider readiness")}
}
