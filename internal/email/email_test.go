package email

import (
	"strings"
	"testing"
)

func TestNewService(t *testing.T) {
	cfg := Config{
		SMTPHost:     "localhost",
		SMTPPort:     1025,
		SMTPUsername: "",
		SMTPPassword: "",
		FromAddress:  "test@example.com",
		FromName:     "Test",
		BaseURL:      "http://localhost:8080",
	}

	svc := NewService(cfg)
	if svc == nil {
		t.Error("NewService() returned nil")
	}
}

func TestEmailTemplates(t *testing.T) {
	cfg := Config{
		SMTPHost:    "localhost",
		SMTPPort:    1025,
		FromAddress: "test@example.com",
		FromName:    "File Processor",
		BaseURL:     "http://localhost:8080",
	}
	svc := NewService(cfg)

	t.Run("verification email template renders", func(t *testing.T) {
		data := VerificationEmailData{
			EmailData: EmailData{
				RecipientName: "John Doe",
				BaseURL:       cfg.BaseURL,
				Year:          2024,
			},
			VerificationURL: "http://localhost:8080/verify?token=abc123",
		}

		html, err := svc.renderTemplate(verificationEmailTemplate, data)
		if err != nil {
			t.Fatalf("renderTemplate() error = %v", err)
		}

		// Check template contains expected content
		checks := []string{
			"John Doe",
			"Verify your email",
			"http://localhost:8080/verify?token=abc123",
			"File Processor",
			"2024",
		}
		for _, check := range checks {
			if !strings.Contains(html, check) {
				t.Errorf("template missing %q", check)
			}
		}

		// Check Nord colors are present
		nordColors := []string{"#2E3440", "#3B4252", "#88C0D0", "#ECEFF4"}
		for _, color := range nordColors {
			if !strings.Contains(html, color) {
				t.Errorf("template missing Nord color %s", color)
			}
		}
	})

	t.Run("password reset email template renders", func(t *testing.T) {
		data := PasswordResetEmailData{
			EmailData: EmailData{
				RecipientName: "Jane Doe",
				BaseURL:       cfg.BaseURL,
				Year:          2024,
			},
			ResetURL: "http://localhost:8080/reset?token=xyz789",
		}

		html, err := svc.renderTemplate(passwordResetEmailTemplate, data)
		if err != nil {
			t.Fatalf("renderTemplate() error = %v", err)
		}

		checks := []string{
			"Jane Doe",
			"Reset your password",
			"http://localhost:8080/reset?token=xyz789",
			"1 hour",
		}
		for _, check := range checks {
			if !strings.Contains(html, check) {
				t.Errorf("template missing %q", check)
			}
		}
	})

	t.Run("welcome email template renders", func(t *testing.T) {
		data := WelcomeEmailData{
			EmailData: EmailData{
				RecipientName: "New User",
				BaseURL:       cfg.BaseURL,
				Year:          2024,
			},
			DashboardURL: "http://localhost:8080/dashboard",
		}

		html, err := svc.renderTemplate(welcomeEmailTemplate, data)
		if err != nil {
			t.Fatalf("renderTemplate() error = %v", err)
		}

		checks := []string{
			"New User",
			"Welcome",
			"http://localhost:8080/dashboard",
			"File Processor",
		}
		for _, check := range checks {
			if !strings.Contains(html, check) {
				t.Errorf("template missing %q", check)
			}
		}
	})
}

func TestEmailDataStructures(t *testing.T) {
	t.Run("EmailData has all fields", func(t *testing.T) {
		data := EmailData{
			RecipientName: "Test",
			BaseURL:       "http://localhost",
			Year:          2024,
		}
		if data.RecipientName != "Test" {
			t.Error("RecipientName not set")
		}
		if data.BaseURL != "http://localhost" {
			t.Error("BaseURL not set")
		}
		if data.Year != 2024 {
			t.Error("Year not set")
		}
	})

	t.Run("VerificationEmailData embeds EmailData", func(t *testing.T) {
		data := VerificationEmailData{
			EmailData: EmailData{
				RecipientName: "Test",
			},
			VerificationURL: "http://verify",
		}
		if data.RecipientName != "Test" {
			t.Error("embedded RecipientName not accessible")
		}
		if data.VerificationURL != "http://verify" {
			t.Error("VerificationURL not set")
		}
	})
}

func TestTemplateValidHTML(t *testing.T) {
	cfg := Config{
		SMTPHost:    "localhost",
		SMTPPort:    1025,
		FromAddress: "test@example.com",
		FromName:    "Test",
		BaseURL:     "http://localhost:8080",
	}
	svc := NewService(cfg)

	templates := []struct {
		name     string
		template string
		data     interface{}
	}{
		{
			"verification",
			verificationEmailTemplate,
			VerificationEmailData{
				EmailData:       EmailData{RecipientName: "Test", Year: 2024},
				VerificationURL: "http://test",
			},
		},
		{
			"password_reset",
			passwordResetEmailTemplate,
			PasswordResetEmailData{
				EmailData: EmailData{RecipientName: "Test", Year: 2024},
				ResetURL:  "http://test",
			},
		},
		{
			"welcome",
			welcomeEmailTemplate,
			WelcomeEmailData{
				EmailData:    EmailData{RecipientName: "Test", Year: 2024},
				DashboardURL: "http://test",
			},
		},
	}

	for _, tt := range templates {
		t.Run(tt.name+" has valid HTML structure", func(t *testing.T) {
			html, err := svc.renderTemplate(tt.template, tt.data)
			if err != nil {
				t.Fatalf("renderTemplate() error = %v", err)
			}

			// Check basic HTML structure
			if !strings.Contains(html, "<!DOCTYPE html>") {
				t.Error("missing DOCTYPE")
			}
			if !strings.Contains(html, "<html>") {
				t.Error("missing html tag")
			}
			if !strings.Contains(html, "</html>") {
				t.Error("missing closing html tag")
			}
			if !strings.Contains(html, "<body") {
				t.Error("missing body tag")
			}
			if !strings.Contains(html, "</body>") {
				t.Error("missing closing body tag")
			}
		})
	}
}
