// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"testing"

	"worker_server/core/domain"
	"worker_server/core/port/out"
)

// TestRFCScoreClassifier tests the RFC header-based classification.
func TestRFCScoreClassifier(t *testing.T) {
	classifier := NewRFCScoreClassifier()

	tests := []struct {
		name           string
		input          *ScoreClassifierInput
		wantCategory   domain.EmailCategory
		wantMinScore   float64
		wantClassified bool
		wantSource     string
	}{
		{
			name: "List-Unsubscribe header should classify as Newsletter",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "newsletter@example.com",
					Subject:   "Weekly Newsletter",
				},
				Headers: &out.ProviderClassificationHeaders{
					ListUnsubscribe: "<mailto:unsubscribe@example.com>",
				},
			},
			wantCategory:   domain.CategoryNewsletter,
			wantMinScore:   0.90,
			wantClassified: true,
			wantSource:     "rfc:list-unsubscribe",
		},
		{
			name: "Precedence: bulk should classify as Marketing",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "promo@store.com",
					Subject:   "50% off sale!",
				},
				Headers: &out.ProviderClassificationHeaders{
					Precedence: "bulk",
				},
			},
			wantCategory:   domain.CategoryMarketing,
			wantMinScore:   0.85,
			wantClassified: true,
			wantSource:     "rfc:precedence-bulk",
		},
		{
			name: "Auto-Submitted header should classify as Notification",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "system@company.com",
					Subject:   "Your report is ready",
				},
				Headers: &out.ProviderClassificationHeaders{
					AutoSubmitted: "auto-generated",
				},
			},
			wantCategory:   domain.CategoryNotification,
			wantMinScore:   0.88,
			wantClassified: true,
			wantSource:     "rfc:auto-submitted",
		},
		{
			name: "SendGrid ESP should classify as Marketing",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "updates@service.com",
					Subject:   "Product Updates",
				},
				Headers: &out.ProviderClassificationHeaders{
					IsSendGrid: true,
				},
			},
			wantCategory:   domain.CategoryMarketing,
			wantMinScore:   0.85,
			wantClassified: true,
			wantSource:     "rfc:esp-esp-sendgrid",
		},
		{
			name: "Mailchimp ESP should classify as Marketing",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "newsletter@brand.com",
					Subject:   "New collection",
				},
				Headers: &out.ProviderClassificationHeaders{
					IsMailchimp: true,
				},
			},
			wantCategory:   domain.CategoryMarketing,
			wantMinScore:   0.88,
			wantClassified: true,
			wantSource:     "rfc:esp-esp-mailchimp",
		},
		{
			name: "noreply@ sender should classify as Notification",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "noreply@github.com",
					Subject:   "New comment on your PR",
				},
				Headers: &out.ProviderClassificationHeaders{},
			},
			wantCategory:   domain.CategoryNotification,
			wantMinScore:   0.65,
			wantClassified: true,
			wantSource:     "rfc:noreply",
		},
		{
			name: "X-Mailer with HubSpot should classify as Marketing",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "sales@company.com",
					Subject:   "Special Offer",
				},
				Headers: &out.ProviderClassificationHeaders{
					XMailer: "HubSpot Email Marketing",
				},
			},
			wantCategory:   domain.CategoryMarketing,
			wantMinScore:   0.85,
			wantClassified: true,
			wantSource:     "rfc:mailer-hubspot",
		},
		{
			name: "No headers should not classify",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "friend@gmail.com",
					Subject:   "Hey, how are you?",
				},
				Headers: nil,
			},
			wantClassified: false,
		},
		{
			name: "Empty headers should not classify",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "colleague@work.com",
					Subject:   "Meeting tomorrow",
				},
				Headers: &out.ProviderClassificationHeaders{},
			},
			wantClassified: false,
		},
		{
			name: "Personal email from gmail should not classify (no signals)",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "john.doe@gmail.com",
					Subject:   "Re: Dinner plans",
				},
				Headers: &out.ProviderClassificationHeaders{},
			},
			wantClassified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := classifier.Classify(context.Background(), tt.input)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.wantClassified {
				if result == nil {
					t.Errorf("expected classification result, got nil")
					return
				}
				if result.Category != tt.wantCategory {
					t.Errorf("category = %v, want %v", result.Category, tt.wantCategory)
				}
				if result.Score < tt.wantMinScore {
					t.Errorf("score = %v, want >= %v", result.Score, tt.wantMinScore)
				}
				if result.Source != tt.wantSource {
					t.Errorf("source = %v, want %v", result.Source, tt.wantSource)
				}
				if result.LLMUsed {
					t.Errorf("LLMUsed = true, want false for RFC classifier")
				}
			} else {
				if result != nil {
					t.Errorf("expected nil result for non-classified email, got category=%v, score=%v, source=%v",
						result.Category, result.Score, result.Source)
				}
			}
		})
	}
}

// TestScorePipelineWithRFCOnly tests the pipeline with only RFC classifier.
func TestScorePipelineWithRFCOnly(t *testing.T) {
	// Create pipeline with minimal dependencies (only RFC classifier works)
	deps := &ScorePipelineDeps{}
	config := &ScorePipelineConfig{
		EarlyExitThreshold:     0.85,
		LLMFallbackThreshold:   0.70,
		EnableSemanticCache:    false,
		EnableAutoLabeling:     false,
		SemanticCacheThreshold: 0.90,
	}

	pipeline := NewScorePipeline(deps, config)

	tests := []struct {
		name         string
		input        *ScoreClassifierInput
		wantCategory domain.EmailCategory
		wantLLMUsed  bool
	}{
		{
			name: "Newsletter with List-Unsubscribe - early exit at RFC",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "news@company.com",
					Subject:   "Weekly Digest",
				},
				Headers: &out.ProviderClassificationHeaders{
					ListUnsubscribe: "<mailto:unsub@company.com>",
				},
			},
			wantCategory: domain.CategoryNewsletter,
			wantLLMUsed:  false,
		},
		{
			name: "Marketing email with Mailchimp - early exit at RFC",
			input: &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "promo@brand.com",
					Subject:   "New Products",
				},
				Headers: &out.ProviderClassificationHeaders{
					IsMailchimp: true,
				},
			},
			wantCategory: domain.CategoryMarketing,
			wantLLMUsed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := pipeline.Classify(context.Background(), tt.input)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected result, got nil")
				return
			}

			if result.Category != tt.wantCategory {
				t.Errorf("category = %v, want %v", result.Category, tt.wantCategory)
			}

			if result.LLMUsed != tt.wantLLMUsed {
				t.Errorf("LLMUsed = %v, want %v", result.LLMUsed, tt.wantLLMUsed)
			}

			t.Logf("Result: category=%v, score=%.2f, source=%s, stage=%s, llmUsed=%v",
				result.Category, result.Score, result.Source, result.Stage, result.LLMUsed)
		})
	}
}

// TestNoReplyPatterns tests various no-reply email patterns.
func TestNoReplyPatterns(t *testing.T) {
	classifier := NewRFCScoreClassifier()

	noReplyEmails := []string{
		"noreply@github.com",
		"no-reply@notifications.google.com",
		"donotreply@bank.com",
		"do-not-reply@service.com",
		"mailer-daemon@mail.google.com",
		"notifications@linkedin.com",
		"alert@security.company.com",
	}

	for _, email := range noReplyEmails {
		t.Run(email, func(t *testing.T) {
			input := &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: email,
					Subject:   "Test notification",
				},
				Headers: &out.ProviderClassificationHeaders{},
			}

			result, err := classifier.Classify(context.Background(), input)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected classification for %s, got nil", email)
				return
			}

			if result.Category != domain.CategoryNotification {
				t.Errorf("email %s: category = %v, want Notification", email, result.Category)
			}

			t.Logf("%s -> category=%v, score=%.2f, source=%s",
				email, result.Category, result.Score, result.Source)
		})
	}
}

// TestRealWorldNoReplyEmails tests classification of actual no-reply emails from production.
func TestRealWorldNoReplyEmails(t *testing.T) {
	classifier := NewRFCScoreClassifier()

	// 실제 프로덕션에서 "other"로 잘못 분류된 이메일들
	realWorldEmails := []struct {
		from           string
		expectedCat    domain.EmailCategory
		shouldClassify bool
	}{
		{"no-reply@twitch.tv", domain.CategoryNotification, true},
		{"noreply@redditmail.com", domain.CategoryNotification, true},
		{"noreply@e.coupang.com", domain.CategoryNotification, true},
		{"notifications@vercel.com", domain.CategoryNotification, true},
		{"naverpayadmin_noreply@navercorp.com", domain.CategoryNotification, true},
		{"noreply@medium.com", domain.CategoryNotification, true},
		{"noreply@github.com", domain.CategoryNotification, true},
		{"googleplay-noreply@google.com", domain.CategoryNotification, true},
		{"no-reply@kaggle.com", domain.CategoryNotification, true},
		{"notifications-noreply@linkedin.com", domain.CategoryNotification, true},
		{"no-reply@accounts.google.com", domain.CategoryNotification, true},
		{"noreply@supabase.com", domain.CategoryNotification, true},
		{"account-security-noreply@accountprotection.microsoft.com", domain.CategoryNotification, true},
		// 일반 이메일은 분류 안 됨
		{"john.doe@gmail.com", domain.CategoryOther, false},
		{"support@company.com", domain.CategoryOther, false},
	}

	classifiedCount := 0
	for _, tc := range realWorldEmails {
		t.Run(tc.from, func(t *testing.T) {
			input := &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: tc.from,
					Subject:   "Test notification",
				},
				Headers: &out.ProviderClassificationHeaders{},
			}

			result, err := classifier.Classify(context.Background(), input)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tc.shouldClassify {
				if result == nil {
					t.Errorf("expected classification for %s, got nil", tc.from)
					return
				}
				if result.Category != tc.expectedCat {
					t.Errorf("%s: category = %v, want %v", tc.from, result.Category, tc.expectedCat)
				}
				classifiedCount++
				t.Logf("✓ %s -> %v (score=%.2f, source=%s)", tc.from, result.Category, result.Score, result.Source)
			} else {
				if result != nil {
					t.Errorf("expected no classification for %s, got %v", tc.from, result.Category)
				}
			}
		})
	}

	t.Logf("\n=== Summary: %d/%d emails would be classified by RFC (no LLM needed) ===",
		classifiedCount, len(realWorldEmails)-2) // -2 for non-noreply emails
}

// TestESPDetection tests Email Service Provider detection.
func TestESPDetection(t *testing.T) {
	classifier := NewRFCScoreClassifier()

	tests := []struct {
		name    string
		headers *out.ProviderClassificationHeaders
		wantESP bool
	}{
		{
			name:    "SendGrid",
			headers: &out.ProviderClassificationHeaders{IsSendGrid: true},
			wantESP: true,
		},
		{
			name:    "Mailchimp",
			headers: &out.ProviderClassificationHeaders{IsMailchimp: true},
			wantESP: true,
		},
		{
			name:    "Amazon SES",
			headers: &out.ProviderClassificationHeaders{IsAmazonSES: true},
			wantESP: true,
		},
		{
			name:    "Mailgun",
			headers: &out.ProviderClassificationHeaders{IsMailgun: true},
			wantESP: true,
		},
		{
			name:    "Postmark",
			headers: &out.ProviderClassificationHeaders{IsPostmark: true},
			wantESP: true,
		},
		{
			name:    "Campaign",
			headers: &out.ProviderClassificationHeaders{IsCampaign: true},
			wantESP: true,
		},
		{
			name:    "No ESP",
			headers: &out.ProviderClassificationHeaders{},
			wantESP: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &ScoreClassifierInput{
				Email: &domain.Email{
					FromEmail: "test@example.com",
					Subject:   "Test",
				},
				Headers: tt.headers,
			}

			result, _ := classifier.Classify(context.Background(), input)

			if tt.wantESP {
				if result == nil {
					t.Errorf("expected ESP detection for %s", tt.name)
					return
				}
				if result.Category != domain.CategoryMarketing {
					t.Errorf("%s: category = %v, want Marketing", tt.name, result.Category)
				}
			} else {
				if result != nil {
					t.Errorf("expected no classification for %s, got %v", tt.name, result.Category)
				}
			}
		})
	}
}
