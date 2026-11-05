package api

import (
    "context"
    "fmt"
    "sort"
    "strings"
    "time"

    "github.com/mtzanidakis/dodo/internal/i18n"
    "github.com/mtzanidakis/dodo/internal/models"
    "github.com/mtzanidakis/dodo/internal/recurrence"
)

type digestItem struct {
    title string
    priority models.Priority
    due time.Time
}

func dailyDigestItems(tasks []*models.Task, start, end time.Time, loc *time.Location) []digestItem {
    var out []digestItem
    for _, task := range tasks {
        due := task.DueAt.In(loc)
        if task.Recurring() {
            rule := recurrence.Rule{
                Freq:*task.RecurrenceFreq,
                Interval:task.RecurrenceInterval,
                ByDay:parseDigestDays(task.RecurrenceByDay),
            }
            if task.RecurrenceByMonthDay!=nil { rule.ByMonthDay=*task.RecurrenceByMonthDay }
            if task.RecurrenceEndAt!=nil { rule.EndAt=*task.RecurrenceEndAt }
            if due.Before(start) {
                due=recurrence.NextOccurrence(rule,task.DueAt,start.Add(-time.Nanosecond),loc).In(loc)
            }
        }
        if due.IsZero() || due.Before(start) || !due.Before(end) { continue }
        out=append(out,digestItem{title:task.Title,priority:task.Priority,due:due})
    }
    sort.Slice(out,func(i,j int)bool{
        if !out[i].due.Equal(out[j].due){return out[i].due.Before(out[j].due)}
        return out[i].title<out[j].title
    })
    return out
}

func parseDigestDays(s string) []time.Weekday {
    names:=map[string]time.Weekday{
       "SU":time.Sunday,"MO":time.Monday,"TU":time.Tuesday,
       "WE":time.Wednesday,"TH":time.Thursday,"FR":time.Friday,"SA":time.Saturday,
    }
    var days []time.Weekday
    for _,value:=range strings.Split(s,","){
        if day,ok:=names[strings.TrimSpace(value)];ok{days=append(days,day)}
    }
    return days
}

func renderDailyDigest(u *models.User, now time.Time, entries []digestItem) string {
    lang:=string(u.Locale)
    heading:=i18n.T("digest.message_title",lang,now.Format("02/01/2006"))
    var b strings.Builder
    b.WriteString(heading)
    b.WriteString("\n\n")
    for _,entry:=range entries {
        line:=fmt.Sprintf("• %s — %s (%s)\n",
           entry.due.Format("15:04"),entry.title,i18n.T("priority."+string(entry.priority),lang))
        if b.Len()+len(line)>3700 {
            b.WriteString(i18n.T("digest.more",lang))
            break
        }
        b.WriteString(line)
    }
    return strings.TrimRight(b.String(),"\n")
}

func (sc *Scheduler) tickDailyDigest(ctx context.Context, now time.Time) {
    if sc.telegram==nil {return}
    users,err:=sc.store.Users.ListDailyDigestEnabled(ctx)
    if err!=nil{
        sc.logger.Warn("digest user listing failed","error",err)
        return
    }
    for _,user:=range users {
        loc,err:=time.LoadLocation(user.Timezone)
        if err!=nil {sc.logger.Warn("digest timezone invalid","user_id",user.ID,"error",err);continue}
        local:=now.In(loc)
        today:=local.Format("2006-01-02")
        if user.DailyDigestLastSentDate==today || local.Format("15:04")<user.DailyDigestTime {continue}
        start:=time.Date(local.Year(),local.Month(),local.Day(),0,0,0,0,loc)
        end:=start.AddDate(0,0,1)
        tasks,err:=sc.store.Tasks.ListCandidatesForDigest(ctx,user.ID,start.UTC(),end.UTC())
        if err!=nil {sc.logger.Warn("digest task listing failed","user_id",user.ID,"error",err);continue}
        items:=dailyDigestItems(tasks,start,end,loc)
        if len(items)==0 {continue}
        message:=renderDailyDigest(user,local,items)
        sendCtx,cancel:=context.WithTimeout(ctx,20*time.Second)
        err=sc.telegram.SendTest(sendCtx,user.ID,user.TelegramChatID,message)
        cancel()
        if err!=nil{
            sc.logger.Warn("telegram daily digest failed","user_id",user.ID,"error",err)
            continue
        }
        if err:=sc.store.Users.MarkDailyDigestSent(ctx,user.ID,today);err!=nil{
            sc.logger.Warn("digest sent but stamp failed","user_id",user.ID,"error",err)
        }
    }
}
