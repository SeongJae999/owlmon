package alert

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// EmailConfig는 SMTP 설정입니다.
type EmailConfig struct {
	Host     string // smtp.gmail.com
	Port     string // 587
	Username string // 발신 계정
	Password string // 앱 비밀번호
	From     string // 발신 주소
	To       []string // 수신 주소 목록
}

// SendTo는 지정한 수신자 목록에 이메일을 발송합니다.
func (c *EmailConfig) SendTo(to []string, subject, body string) error {
	orig := c.To
	c.To = to
	err := c.Send(subject, body)
	c.To = orig
	return err
}

// Send는 알림 이메일을 발송합니다.
func (c *EmailConfig) Send(subject, body string) error {
	auth := smtp.PlainAuth("", c.Username, c.Password, c.Host)

	msg := strings.Join([]string{
		"From: OWLmon <" + c.From + ">",
		"To: " + strings.Join(c.To, ", "),
		"Subject: [OWLmon] " + subject,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
		"",
		"---",
		"발송 시각: " + time.Now().Format("2006-01-02 15:04:05"),
		"OWLmon 모니터링 시스템",
	}, "\r\n")

	addr := fmt.Sprintf("%s:%s", c.Host, c.Port)
	return smtp.SendMail(addr, auth, c.From, c.To, []byte(msg))
}
