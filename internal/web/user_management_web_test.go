package web

import (
 "context"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"
 "time"

 "github.com/mtzanidakis/dodo/internal/auth"
 "github.com/mtzanidakis/dodo/internal/models"
)

func TestDefaultDueRoundingAndMonthEnd(t *testing.T) {
 loc,err:=time.LoadLocation("Europe/Rome")
 if err!=nil {t.Fatal(err)}
 now:=time.Date(2026,time.January,31,14,40,0,0,loc)
 for _,tt:=range []struct{choice string;want time.Time}{
   {"now",now},
   {"24h",time.Date(2026,time.February,1,15,0,0,0,loc)},
   {"week",time.Date(2026,time.February,7,15,0,0,0,loc)},
   {"month",time.Date(2026,time.February,28,15,0,0,0,loc)},
 }{
  got:=defaultDue(now,tt.choice)
  if !got.Equal(tt.want) {t.Errorf("%s: got %s want %s",tt.choice,got,tt.want)}
 }
}

func TestAdminPageAndSettings(t *testing.T) {
 mux,st,user,session:=newWebEnv(t)
 request:=httptest.NewRequest("GET","/admin",nil)
 withSession(request,session)
 response:=httptest.NewRecorder()
 mux.ServeHTTP(response,request)
 if response.Code!=http.StatusOK || !strings.Contains(response.Body.String(),"w@example.com") {
    t.Fatalf("admin page status %d",response.Code)
 }
 other:=&models.User{Email:"another@example.com",PasswordHash:"x"}
 if err:=st.Users.Create(context.Background(),other);err!=nil {t.Fatal(err)}
 gen,err:=auth.GenerateSession()
 if err!=nil {t.Fatal(err)}
 if _,err=st.Sessions.Create(context.Background(),other.ID,gen.Hash,"test",time.Hour);err!=nil {t.Fatal(err)}
 request=httptest.NewRequest("GET","/admin",nil)
 withSession(request,gen.Full)
 response=httptest.NewRecorder()
 mux.ServeHTTP(response,request)
 if response.Code!=http.StatusForbidden {t.Fatalf("nonadmin status %d",response.Code)}
 if !user.IsAdmin() {t.Fatal("bootstrap admin user not marked admin")}
}

func TestSaveDefaultsValidation(t *testing.T) {
 mux,st,user,session:=newWebEnv(t)
 form:=url.Values{"due":{"week"},"priority":{"high"},"recurrence":{"weekly"},"recurrence_interval":{"7"}}
 request:=httptest.NewRequest("POST","/account/defaults",strings.NewReader(form.Encode()))
 request.Header.Set("Content-Type","application/x-www-form-urlencoded")
 withSession(request,session)
 token:=auth.IssueCSRF(httptest.NewRecorder())
 request.AddCookie(&http.Cookie{Name:auth.CSRFCookie,Value:token})
 form.Set("_csrf",token)
 request=httptest.NewRequest("POST","/account/defaults",strings.NewReader(form.Encode()))
 request.Header.Set("Content-Type","application/x-www-form-urlencoded")
 withSession(request,session)
 request.AddCookie(&http.Cookie{Name:auth.CSRFCookie,Value:token})
 response:=httptest.NewRecorder()
 mux.ServeHTTP(response,request)
 if response.Code!=http.StatusSeeOther {t.Fatalf("status %d",response.Code)}
 updated,err:=st.Users.GetByID(context.Background(),user.ID)
 if err!=nil || updated.DefaultDue!="week" || updated.DefaultRecurrenceInterval!=7 {
    t.Fatalf("saved defaults: %v",err)
 }
}
