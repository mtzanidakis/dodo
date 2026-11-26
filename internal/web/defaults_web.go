package web

import (
 "encoding/json"
 "net/http"
 "net/url"
 "strconv"
 "time"

 "github.com/mtzanidakis/dodo/internal/auth"
 "github.com/mtzanidakis/dodo/internal/i18n"
 "github.com/mtzanidakis/dodo/internal/models"
)

func (h *Handler) handleDefaults(w http.ResponseWriter,r *http.Request) {
 u:=auth.UserFromContext(r.Context())
 interval,err:=strconv.Atoi(r.FormValue("recurrence_interval"))
 if err!=nil { interval=0 }
 err=h.deps.Store.Users.SetDefaults(r.Context(),u.ID,r.FormValue("due"),
   models.Priority(r.FormValue("priority")),r.FormValue("recurrence"),interval)
 if err!=nil {
    http.Redirect(w,r,"/account?err="+url.QueryEscape(i18n.T("error.invalid",string(u.Locale))),http.StatusSeeOther);return
 }
 http.Redirect(w,r,"/account?ok="+url.QueryEscape(i18n.T("defaults.saved",string(u.Locale))),http.StatusSeeOther)
}

func defaultDue(now time.Time,choice string)time.Time {
 switch choice {
 case "now":
    return now
 case "week":
    return now.AddDate(0,0,7).Round(time.Hour)
 case "month":
    year,month,day:=now.Date()
    nextFirst:=time.Date(year,month+1,1,now.Hour(),now.Minute(),now.Second(),0,now.Location())
    nextLast:=nextFirst.AddDate(0,1,-1).Day()
    if day>nextLast { day=nextLast }
    return time.Date(nextFirst.Year(),nextFirst.Month(),day,now.Hour(),now.Minute(),now.Second(),0,now.Location()).Round(time.Hour)
 default:
    return now.Add(24*time.Hour).Round(time.Hour)
 }
}

func (h *Handler) handleTaskDefaults(w http.ResponseWriter,r *http.Request) {
 u:=auth.UserFromContext(r.Context())
 loc:=loadLoc(u.Timezone)
 due:=defaultDue(time.Now().In(loc),u.DefaultDue)
 value:=due.Format("2006-01-02T15:04")
 if u.DateFormat!="" {
    value=fmtDateTime(due,u.DateFormat,"2006-01-02T15:04")
 }
 freq:=u.DefaultRecurrence
 if freq=="once" { freq="" }
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("Content-Type","application/json")
 _=json.NewEncoder(w).Encode(map[string]any{
    "due":value,"priority":u.DefaultPriority,"recurrence":freq,
    "interval":u.DefaultRecurrenceInterval,
 })
}
