package email

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"

	"github.com/abdul-hamid-achik/file-processor/internal/logger"
)

// Config holds email service configuration.
type Config struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	FromAddress  string
	FromName     string
	BaseURL      string // For links in emails
}

// Service handles sending emails.
type Service struct {
	cfg Config
}

// NewService creates a new email service.
func NewService(cfg Config) *Service {
	return &Service{cfg: cfg}
}

// EmailData contains common email template data.
type EmailData struct {
	RecipientName string
	BaseURL       string
	Year          int
}

// VerificationEmailData contains data for the email verification email.
type VerificationEmailData struct {
	EmailData
	VerificationURL string
}

// PasswordResetEmailData contains data for the password reset email.
type PasswordResetEmailData struct {
	EmailData
	ResetURL string
}

// WelcomeEmailData contains data for the welcome email.
type WelcomeEmailData struct {
	EmailData
	DashboardURL string
}

// Send sends an email with the given subject and body.
func (s *Service) Send(to, subject, htmlBody string) error {
	log := logger.Default()

	from := fmt.Sprintf("%s <%s>", s.cfg.FromName, s.cfg.FromAddress)

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", from, to, subject, htmlBody)

	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	var auth smtp.Auth
	if s.cfg.SMTPUsername != "" && s.cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	}

	err := smtp.SendMail(addr, auth, s.cfg.FromAddress, []string{to}, []byte(msg))
	if err != nil {
		log.Error("email send failed", "to", to, "subject", subject, "error", err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Info("email sent", "to", to, "subject", subject)
	return nil
}

// SendVerificationEmail sends an email verification email.
func (s *Service) SendVerificationEmail(to, name, token string) error {
	data := VerificationEmailData{
		EmailData: EmailData{
			RecipientName: name,
			BaseURL:       s.cfg.BaseURL,
			Year:          2024,
		},
		VerificationURL: fmt.Sprintf("%s/verify-email?token=%s", s.cfg.BaseURL, token),
	}

	html, err := s.renderTemplate(verificationEmailTemplate, data)
	if err != nil {
		return err
	}

	return s.Send(to, "Verify your email address", html)
}

// SendPasswordResetEmail sends a password reset email.
func (s *Service) SendPasswordResetEmail(to, name, token string) error {
	data := PasswordResetEmailData{
		EmailData: EmailData{
			RecipientName: name,
			BaseURL:       s.cfg.BaseURL,
			Year:          2024,
		},
		ResetURL: fmt.Sprintf("%s/reset-password?token=%s", s.cfg.BaseURL, token),
	}

	html, err := s.renderTemplate(passwordResetEmailTemplate, data)
	if err != nil {
		return err
	}

	return s.Send(to, "Reset your password", html)
}

// SendWelcomeEmail sends a welcome email to new users.
func (s *Service) SendWelcomeEmail(to, name string) error {
	data := WelcomeEmailData{
		EmailData: EmailData{
			RecipientName: name,
			BaseURL:       s.cfg.BaseURL,
			Year:          2024,
		},
		DashboardURL: fmt.Sprintf("%s/dashboard", s.cfg.BaseURL),
	}

	html, err := s.renderTemplate(welcomeEmailTemplate, data)
	if err != nil {
		return err
	}

	return s.Send(to, "Welcome to File Processor!", html)
}

func (s *Service) renderTemplate(tmplStr string, data any) (string, error) {
	tmpl, err := template.New("email").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// Email templates with Nord theme colors
const verificationEmailTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #2E3440;">
    <table role="presentation" style="width: 100%; border-collapse: collapse;">
        <tr>
            <td style="padding: 40px 20px;">
                <table role="presentation" style="max-width: 600px; margin: 0 auto; background-color: #3B4252; border-radius: 8px; overflow: hidden;">
                    <tr>
                        <td style="padding: 40px; text-align: center; background-color: #434C5E;">
                            <h1 style="margin: 0; color: #88C0D0; font-size: 24px;">File Processor</h1>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 40px;">
                            <h2 style="margin: 0 0 20px; color: #ECEFF4; font-size: 20px;">Verify your email address</h2>
                            <p style="margin: 0 0 20px; color: #D8DEE9; line-height: 1.6;">
                                Hi {{.RecipientName}},
                            </p>
                            <p style="margin: 0 0 30px; color: #D8DEE9; line-height: 1.6;">
                                Please click the button below to verify your email address and complete your registration.
                            </p>
                            <table role="presentation" style="margin: 0 auto;">
                                <tr>
                                    <td style="border-radius: 4px; background-color: #88C0D0;">
                                        <a href="{{.VerificationURL}}" style="display: inline-block; padding: 14px 28px; color: #2E3440; text-decoration: none; font-weight: 600;">
                                            Verify Email
                                        </a>
                                    </td>
                                </tr>
                            </table>
                            <p style="margin: 30px 0 0; color: #4C566A; font-size: 14px; line-height: 1.6;">
                                If you didn't create an account, you can safely ignore this email.
                            </p>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 20px 40px; background-color: #434C5E; text-align: center;">
                            <p style="margin: 0; color: #4C566A; font-size: 12px;">
                                &copy; {{.Year}} File Processor. All rights reserved.
                            </p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`

const passwordResetEmailTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #2E3440;">
    <table role="presentation" style="width: 100%; border-collapse: collapse;">
        <tr>
            <td style="padding: 40px 20px;">
                <table role="presentation" style="max-width: 600px; margin: 0 auto; background-color: #3B4252; border-radius: 8px; overflow: hidden;">
                    <tr>
                        <td style="padding: 40px; text-align: center; background-color: #434C5E;">
                            <h1 style="margin: 0; color: #88C0D0; font-size: 24px;">File Processor</h1>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 40px;">
                            <h2 style="margin: 0 0 20px; color: #ECEFF4; font-size: 20px;">Reset your password</h2>
                            <p style="margin: 0 0 20px; color: #D8DEE9; line-height: 1.6;">
                                Hi {{.RecipientName}},
                            </p>
                            <p style="margin: 0 0 30px; color: #D8DEE9; line-height: 1.6;">
                                We received a request to reset your password. Click the button below to choose a new password.
                            </p>
                            <table role="presentation" style="margin: 0 auto;">
                                <tr>
                                    <td style="border-radius: 4px; background-color: #88C0D0;">
                                        <a href="{{.ResetURL}}" style="display: inline-block; padding: 14px 28px; color: #2E3440; text-decoration: none; font-weight: 600;">
                                            Reset Password
                                        </a>
                                    </td>
                                </tr>
                            </table>
                            <p style="margin: 30px 0 0; color: #4C566A; font-size: 14px; line-height: 1.6;">
                                This link will expire in 1 hour. If you didn't request a password reset, you can safely ignore this email.
                            </p>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 20px 40px; background-color: #434C5E; text-align: center;">
                            <p style="margin: 0; color: #4C566A; font-size: 12px;">
                                &copy; {{.Year}} File Processor. All rights reserved.
                            </p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`

const welcomeEmailTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #2E3440;">
    <table role="presentation" style="width: 100%; border-collapse: collapse;">
        <tr>
            <td style="padding: 40px 20px;">
                <table role="presentation" style="max-width: 600px; margin: 0 auto; background-color: #3B4252; border-radius: 8px; overflow: hidden;">
                    <tr>
                        <td style="padding: 40px; text-align: center; background-color: #434C5E;">
                            <h1 style="margin: 0; color: #88C0D0; font-size: 24px;">File Processor</h1>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 40px;">
                            <h2 style="margin: 0 0 20px; color: #ECEFF4; font-size: 20px;">Welcome aboard!</h2>
                            <p style="margin: 0 0 20px; color: #D8DEE9; line-height: 1.6;">
                                Hi {{.RecipientName}},
                            </p>
                            <p style="margin: 0 0 30px; color: #D8DEE9; line-height: 1.6;">
                                Thanks for joining File Processor! You can now upload and process your files with ease.
                            </p>
                            <table role="presentation" style="margin: 0 auto;">
                                <tr>
                                    <td style="border-radius: 4px; background-color: #A3BE8C;">
                                        <a href="{{.DashboardURL}}" style="display: inline-block; padding: 14px 28px; color: #2E3440; text-decoration: none; font-weight: 600;">
                                            Go to Dashboard
                                        </a>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 20px 40px; background-color: #434C5E; text-align: center;">
                            <p style="margin: 0; color: #4C566A; font-size: 12px;">
                                &copy; {{.Year}} File Processor. All rights reserved.
                            </p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`
