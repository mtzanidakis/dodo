package web

import (
 "context"
 "crypto/tls"
 "fmt"
 "mime"
 "net"
 "net/mail"
 "net/smtp"
 "net/url"
 "strconv"
 "strings"
 "time"
)

type MailSettings struct {
 Host string
 Port string
 User string
 Password string
 From string
 Mode string
 PublicURL string
}

func (m MailSettings) Ready() bool {
 u, err := url.Parse(m.PublicURL)
 if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil { return false }
 from, err := mail.ParseAddress(m.From)
 return err == nil && from.Address == m.From && m.Host != "" && m.Port != ""
}

func (m MailSettings) SendVerification(ctx context.Context, recipient, token, lang string) error {
 if !m.Ready() { return fmt.Errorf("smtp registration is not configured") }
 to, err := mail.ParseAddress(recipient)
 if err != nil || to.Address != recipient || strings.ContainsAny(recipient,"\r\n") { return fmt.Errorf("invalid recipient") }
 rawPort, err := strconv.Atoi(m.Port)
 if err != nil || rawPort < 1 || rawPort > 65535 { return fmt.Errorf("invalid smtp port") }
 link := strings.TrimRight(m.PublicURL,"/")+"/verify?token="+url.QueryEscape(token)+"&lang="+url.QueryEscape(lang)
 subject := "Verify your Dodo email"
 instruction := "Open this link to verify your email address and activate your Dodo account:"
 expiration := "This link expires in 24 hours."
 switch lang {
 case "it":
   subject="Verifica il tuo indirizzo email per Dodo"
   instruction="Apri questo collegamento per verificare l'indirizzo email e attivare il tuo account Dodo:"
   expiration="Questo collegamento scade dopo 24 ore."
 case "el":
   subject="Επιβεβαίωση email για το Dodo"
   instruction="Ανοίξτε τον σύνδεσμο για να επιβεβαιώσετε το email και να ενεργοποιήσετε τον λογαριασμό σας:"
   expiration="Ο σύνδεσμος λήγει σε 24 ώρες."
 }
 body := "From: "+m.From+"\r\nTo: "+recipient+"\r\nSubject: "+mime.QEncoding.Encode("utf-8",subject)+"\r\n"+
  "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n"+
  instruction+"\r\n"+link+"\r\n\r\n"+expiration+"\r\n"
 addr := net.JoinHostPort(m.Host,m.Port)
 dialer := &net.Dialer{Timeout:10*time.Second}
 conn,err := dialer.DialContext(ctx,"tcp",addr)
 if err != nil { return err }
 defer func(){ _ = conn.Close() }()
 _ = conn.SetDeadline(time.Now().Add(30*time.Second))
 tlsCfg := &tls.Config{ServerName:m.Host,MinVersion:tls.VersionTLS12}
 var client *smtp.Client
 if m.Mode == "tls" {
   secure := tls.Client(conn,tlsCfg)
   if err = secure.HandshakeContext(ctx); err != nil { return err }
   client,err = smtp.NewClient(secure,m.Host)
   if err != nil { return err }
 } else if m.Mode == "starttls" {
   client,err = smtp.NewClient(conn,m.Host)
   if err != nil { return err }
   if ok,_ := client.Extension("STARTTLS"); !ok { _ = client.Close();return fmt.Errorf("smtp server does not support STARTTLS") }
   if err = client.StartTLS(tlsCfg); err != nil { _ = client.Close();return err }
 } else {
   return fmt.Errorf("smtp mode must be tls or starttls")
 }
 defer func(){ _ = client.Close() }()
 if m.User != "" {
   if err := client.Auth(smtp.PlainAuth("",m.User,m.Password,m.Host)); err != nil { return err }
 }
 if err := client.Mail(m.From); err != nil { return err }
 if err := client.Rcpt(recipient); err != nil { return err }
 w,err := client.Data()
 if err != nil { return err }
 if _,err = w.Write([]byte(body)); err != nil { _ = w.Close();return err }
 if err = w.Close(); err != nil { return err }
 _ = client.Quit()
 return nil
}
