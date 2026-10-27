package project

import (
 "encoding/json"
 "os"
 "reflect"
 "strings"
 "testing"
)

func TestSharedFixtures(t *testing.T) {
 data, err := os.ReadFile("../../contracts/project.fixtures.json")
 if err != nil { t.Fatal(err) }
 var cases []struct {
  Name string `json:"name"`
  Input Input `json:"input"`
  Normalized Input `json:"normalized"`
  Errors []FieldError `json:"errors"`
 }
 if err := json.Unmarshal(data, &cases); err != nil { t.Fatal(err) }
 for _, tc := range cases { t.Run(tc.Name, func(t *testing.T) {
  normalized, errs := Validate(tc.Input)
  if normalized != tc.Normalized { t.Errorf("normalized: got %#v want %#v", normalized, tc.Normalized) }
  if !reflect.DeepEqual(errs, tc.Errors) { t.Errorf("errors: got %#v want %#v", errs, tc.Errors) }
 }) }
}

func TestBoundaries(t *testing.T) {
 for _, field := range []struct{name string; limit int}{ {"name",120}, {"goal",2000}, {"description",10000} } {
  for _, delta := range []int{-1,0,1} {
   t.Run(field.name+string(rune('b'+delta)), func(t *testing.T) {
    in := Input{Name:"Проект",Goal:"Цель"}
    value := strings.Repeat("🚀",field.limit+delta)
    switch field.name {case "name": in.Name=value; case "goal":in.Goal=value; case "description":in.Description=value}
    _, errs := Validate(in)
    if delta <= 0 && len(errs)!=0 { t.Fatalf("at or below limit: %v",errs) }
    if delta > 0 && (len(errs)!=1 || errs[0].Code!="MAX_LENGTH" || errs[0].Params["max"]!=field.limit) { t.Fatalf("over limit: %v",errs) }
   })
  }
 }
}

func TestRejectInvalidUTF8(t *testing.T) {
 _, errs := Validate(Input{Name:string([]byte{0xff}),Goal:"Goal"})
 if len(errs)!=1 || errs[0].Code!="INVALID_UTF8" {t.Fatalf("invalid encoding accepted: %v",errs)}
}

func TestNoUnicodeNormalization(t *testing.T) {
 in:=Input{Name:"e\u0301",Goal:"Goal"}
 got,errs:=Validate(in)
 if got!=in || len(errs)!=0 {t.Fatalf("unexpected normalization: %#v %v",got,errs)}
}
