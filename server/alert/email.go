package alert

import (
	"crypto/tls"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// smtp.SendMail은 타임아웃이 없어 SMTP 서버가 멈추면 알림 루프 전체가 멈춤
const smtpTimeout = 30 * time.Second

// EmailConfig는 SMTP 설정입니다.
type EmailConfig struct {
	Host     string   // smtp.gmail.com
	Port     string   // 587
	Username string   // 발신 계정
	Password string   // 앱 비밀번호
	From     string   // 발신 주소
	To       []string // 수신 주소 목록
}

// Send는 기본 수신자(To)에게 알림 이메일을 발송합니다.
func (c *EmailConfig) Send(subject, body string) error {
	return c.SendTo(c.To, subject, body)
}

// SendTo는 지정한 수신자 목록에 이메일을 발송합니다. 공유 설정(c.To)은 변경하지 않습니다.
func (c *EmailConfig) SendTo(to []string, subject, body string) error {
	msg := strings.Join([]string{
		"From: OWLmon <" + c.From + ">",
		"To: " + strings.Join(to, ", "),
		"Subject: [OWLmon] " + subject,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
		"",
		"---",
		"발송 시각: " + time.Now().Format("2006-01-02 15:04:05"),
		"OWLmon 모니터링 시스템",
	}, "\r\n")

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(c.Host, c.Port), smtpTimeout)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer cl.Close()

	// 이하 smtp.SendMail과 동일한 절차
	if ok, _ := cl.Extension("STARTTLS"); ok {
		if err := cl.StartTLS(&tls.Config{ServerName: c.Host}); err != nil {
			return err
		}
	}
	if ok, _ := cl.Extension("AUTH"); ok && c.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return err
		}
	}
	if err := cl.Mail(c.From); err != nil {
		return err
	}
	for _, addr := range to {
		if err := cl.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}
