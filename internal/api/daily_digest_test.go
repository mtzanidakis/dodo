package api

import (
    "context"
    "log/slog"
    "strings"
    "testing"
    "time"

    "github.com/mtzanidakis/dodo/internal/db"
    "github.com/mtzanidakis/dodo/internal/models"
    "github.com/mtzanidakis/dodo/internal/store"
    "github.com/mtzanidakis/dodo/internal/ws"
)

type digestTelegramStub struct {
    sent []string
}
func (*digestTelegramStub) ValidateToken(context.Context,string)(string,error){return "",nil}
func (s *digestTelegramStub) SendTest(_ context.Context,_,_,message string)error{
    s.sent=append(s.sent,message)
    return nil
}
func (*digestTelegramStub) SendReminder(context.Context,string,string,string,string,string)error{return nil}
func (*digestTelegramStub) StartForUser(context.Context,string)error{return nil}
func (*digestTelegramStub) StopForUser(string)error{return nil}
func (*digestTelegramStub) StartAll(context.Context)error{return nil}
func (*digestTelegramStub) StopAll(){}

func TestDigestIncludesRecurringAndOnlyToday(t *testing.T) {
    loc,err:=time.LoadLocation("Europe/Rome")
    if err!=nil { t.Fatal(err) }
    start:=time.Date(2026,time.November,5,0,0,0,0,loc)
    end:=start.AddDate(0,0,1)
    daily:=models.FreqDaily
    tasks:=[]*models.Task{
        {Title:"Tomorrow",DueAt:end.Add(time.Hour)},
        {Title:"Yesterday",DueAt:start.Add(-time.Hour)},
        {Title:"Today",DueAt:start.Add(15*time.Hour),Priority:models.PriorityHigh},
        {Title:"Recurring",DueAt:start.AddDate(0,0,-2).Add(9*time.Hour),RecurrenceFreq:&daily,RecurrenceInterval:1},
    }
    items:=dailyDigestItems(tasks,start,end,loc)
    if len(items)!=2 || items[0].title!="Recurring" || items[1].title!="Today" {
        t.Fatalf("unexpected digest items: %#v",items)
    }
    u:=&models.User{Locale:models.LocaleIt}
    message:=renderDailyDigest(u,start,items)
    if !strings.Contains(message,"Recurring") || !strings.Contains(message,"Today") ||
       strings.Contains(message,"Tomorrow") || strings.Contains(message,"Yesterday") {
       t.Fatalf("unexpected message %q",message)
    }
}

func TestDailyDigestOncePerLocalDay(t *testing.T) {
    d,err:=db.Open(":memory:")
    if err!=nil {t.Fatal(err)}
    defer d.Close()
    st:=store.New(d)
    ctx:=context.Background()
    u:=&models.User{Email:"digest@example.org",PasswordHash:"hash",Timezone:"Europe/Rome",Locale:models.LocaleIt}
    if err:=st.Users.Create(ctx,u);err!=nil {t.Fatal(err)}
    now:=time.Date(2026,time.November,5,8,31,0,0,time.FixedZone("CET",3600))
    task:=&models.Task{UserID:u.ID,Title:"Scadenza oggi",Priority:models.PriorityNormal,DueAt:now.Add(4*time.Hour)}
    if err:=st.Tasks.Create(ctx,task);err!=nil {t.Fatal(err)}
    if err:=st.Users.SetDailyDigest(ctx,u.ID,true,"08:30");err!=nil {t.Fatal(err)}
    if err:=st.Users.SetTelegramChatID(ctx,u.ID,"12345","12345");err!=nil {t.Fatal(err)}
    if err:=st.Users.SetTelegramConfig(ctx,u.ID,"token","12345");err!=nil {t.Fatal(err)}
    sender:=&digestTelegramStub{}
    scheduler:=newScheduler(time.Minute,st,ws.NewHub(slog.Default()),sender,slog.Default())
    scheduler.tickDailyDigest(ctx,now)
    scheduler.tickDailyDigest(ctx,now.Add(time.Minute))
    if len(sender.sent)!=1 {t.Fatalf("expected one summary, sent %d",len(sender.sent))}
    if !strings.Contains(sender.sent[0],"Scadenza oggi"){t.Fatal("missing task")}
    later,err:=st.Users.GetByID(ctx,u.ID)
    if err!=nil || later.DailyDigestLastSentDate!="2026-11-05" {t.Fatalf("last sent not persisted: %v",err)}
}

func TestDailyDigestSkippedWhenNoTasksDueToday(t *testing.T) {
    d,err:=db.Open(":memory:")
    if err!=nil {t.Fatal(err)}
    defer d.Close()
    st:=store.New(d)
    ctx:=context.Background()
    u:=&models.User{Email:"no-tasks@example.org",PasswordHash:"hash",Timezone:"Europe/Rome",Locale:models.LocaleIt}
    if err:=st.Users.Create(ctx,u);err!=nil {t.Fatal(err)}
    now:=time.Date(2026,time.November,5,10,0,0,0,time.FixedZone("CET",3600))
    future:=&models.Task{UserID:u.ID,Title:"Domani",Priority:models.PriorityNormal,DueAt:now.AddDate(0,0,1)}
    if err:=st.Tasks.Create(ctx,future);err!=nil {t.Fatal(err)}
    if err:=st.Users.SetDailyDigest(ctx,u.ID,true,"09:00");err!=nil {t.Fatal(err)}
    if err:=st.Users.SetTelegramChatID(ctx,u.ID,"123","123");err!=nil {t.Fatal(err)}
    if err:=st.Users.SetTelegramConfig(ctx,u.ID,"token","123");err!=nil {t.Fatal(err)}
    sender:=&digestTelegramStub{}
    sc:=newScheduler(time.Minute,st,ws.NewHub(slog.Default()),sender,slog.Default())
    sc.tickDailyDigest(ctx,now)
    if len(sender.sent)!=0 {t.Fatalf("should not send without due tasks: %#v",sender.sent)}
}
