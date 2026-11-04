package analysis

import (
 "context"
 "testing"
)

func TestFullQueueWithoutEmbeddingsRegression(t *testing.T) {
 db, _, embed := analysisDB(t)
 gen := &fakeGenerator{profile:"full-queue"}
 s := testService(t,db,nil,gen,"")
 r,err:=s.startAndWait(t,context.Background(),"a","p")
 if err!=nil || r.Status!="completed" || gen.calls.Load()!=1 || embed.calls.Load()!=0 {t.Fatalf("full queue must compare unindexed sources without retrieval: %+v calls=%d err=%v",r,gen.calls.Load(),err)}
}
