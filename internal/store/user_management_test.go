package store_test

import (
 "context"
 "errors"
 "testing"

 "github.com/mtzanidakis/dodo/internal/db"
 "github.com/mtzanidakis/dodo/internal/models"
 "github.com/mtzanidakis/dodo/internal/store"
)

func TestAdminBootstrapAndSafety(t *testing.T) {
 d,err:=db.Open(":memory:")
 if err!=nil { t.Fatal(err) }
 defer d.Close()
 s:=store.New(d)
 ctx:=context.Background()
 first:=&models.User{Email:"first@example.org",PasswordHash:"hash"}
 if err:=s.Users.Create(ctx,first);err!=nil { t.Fatal(err) }
 found,err:=s.Users.GetByID(ctx,first.ID)
 if err!=nil || !found.IsAdmin() { t.Fatalf("first user must be admin: %v",err) }
 if err:=s.Users.SetDisabled(ctx,first.ID,true);!errors.Is(err,models.ErrConflict) { t.Fatalf("last admin disable: %v",err) }
 if err:=s.Users.SetRole(ctx,first.ID,"user");!errors.Is(err,models.ErrConflict) { t.Fatalf("last admin demotion: %v",err) }
 if err:=s.Users.DeletePermanently(ctx,first.ID);!errors.Is(err,models.ErrConflict) { t.Fatalf("last admin deletion: %v",err) }
 second:=&models.User{Email:"second@example.org",PasswordHash:"hash"}
 if err:=s.Users.Create(ctx,second);err!=nil { t.Fatal(err) }
 if err:=s.Users.SetRole(ctx,second.ID,"admin");err!=nil { t.Fatal(err) }
 if err:=s.Users.SetDisabled(ctx,first.ID,true);err!=nil { t.Fatal(err) }
 other,err:=s.Users.GetByID(ctx,first.ID)
 if err!=nil || other.Active() { t.Fatalf("disabled account is active: %v",err) }
 if err:=s.Users.SetDisabled(ctx,first.ID,false);err!=nil { t.Fatal(err) }
 if err:=s.Users.SetRole(ctx,first.ID,"user");err!=nil { t.Fatal(err) }
}

func TestRegistrationVerificationAndDefaults(t *testing.T) {
 d,err:=db.Open(":memory:")
 if err!=nil { t.Fatal(err) }
 defer d.Close()
 s:=store.New(d)
 ctx:=context.Background()
 user:=&models.User{Email:"new@example.org",PasswordHash:"hash"}
 if err:=s.Users.CreatePending(ctx,user,"hashed-token");err!=nil { t.Fatal(err) }
 pending,err:=s.Users.GetByID(ctx,user.ID)
 if err!=nil || pending.Active() { t.Fatalf("pending user active: %v",err) }
 if err:=s.Users.VerifyEmail(ctx,"incorrect");!errors.Is(err,models.ErrNotFound) { t.Fatalf("invalid token: %v",err) }
 if err:=s.Users.VerifyEmail(ctx,"hashed-token");err!=nil { t.Fatal(err) }
 verified,err:=s.Users.GetByID(ctx,user.ID)
 if err!=nil || !verified.IsAdmin() { t.Fatalf("verified first user must be admin: %v",err) }
 if err:=s.Users.VerifyEmail(ctx,"hashed-token");!errors.Is(err,models.ErrNotFound) { t.Fatalf("token reused: %v",err) }
 if err:=s.Users.SetDefaults(ctx,user.ID,"week",models.PriorityHigh,"weekly",7);err!=nil { t.Fatal(err) }
 updated,err:=s.Users.GetByID(ctx,user.ID)
 if err!=nil || updated.DefaultDue!="week" || updated.DefaultRecurrenceInterval!=7 { t.Fatalf("defaults not saved: %v",err) }
 if err:=s.Users.SetDefaults(ctx,user.ID,"week",models.PriorityHigh,"weekly",366);!errors.Is(err,models.ErrValidation) { t.Fatalf("interval validation: %v",err) }
}
