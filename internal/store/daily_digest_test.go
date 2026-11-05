package store_test

import (
    "context"
    "errors"
    "testing"

    "github.com/mtzanidakis/dodo/internal/db"
    "github.com/mtzanidakis/dodo/internal/models"
    "github.com/mtzanidakis/dodo/internal/store"
)

func TestDailyDigestPreferencesValidation(t *testing.T) {
    d,err:=db.Open(":memory:")
    if err!=nil{t.Fatal(err)}
    defer d.Close()
    st:=store.New(d)
    ctx:=context.Background()
    u:=&models.User{Email:"prefs@example.org",PasswordHash:"x"}
    if err:=st.Users.Create(ctx,u);err!=nil{t.Fatal(err)}
    for _,bad:=range []string{"25:00","08:60","8:30","08:30\n",""}{
        if err:=st.Users.SetDailyDigest(ctx,u.ID,true,bad);!errors.Is(err,models.ErrValidation){
            t.Fatalf("expected invalid time %q, got %v",bad,err)
        }
    }
    if err:=st.Users.SetDailyDigest(ctx,u.ID,true,"07:15");err!=nil{t.Fatal(err)}
    got,err:=st.Users.GetByID(ctx,u.ID)
    if err!=nil||!got.DailyDigestEnabled||got.DailyDigestTime!="07:15"{
        t.Fatalf("settings not saved: %v",err)
    }
    if err:=st.Users.SetDailyDigest(ctx,u.ID,false,"07:15");err!=nil{t.Fatal(err)}
    got,err=st.Users.GetByID(ctx,u.ID)
    if err!=nil||got.DailyDigestEnabled{t.Fatalf("settings not disabled: %v",err)}
}
