// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"regexp"
	"strings"

	"worker_server/core/domain"
)

// =============================================================================
// Subject Pattern Score Classifier
// =============================================================================

// SubjectScoreClassifier performs classification based on email subject patterns.
// This classifier recognizes common subject patterns for various email types.
type SubjectScoreClassifier struct {
	patterns []subjectPattern
}

type subjectPattern struct {
	pattern     *regexp.Regexp
	keywords    []string // Simple keyword matching (faster than regex)
	category    domain.EmailCategory
	subCategory domain.EmailSubCategory
	priority    domain.Priority
	score       float64
	source      string
}

// NewSubjectScoreClassifier creates a new subject pattern classifier.
func NewSubjectScoreClassifier() *SubjectScoreClassifier {
	c := &SubjectScoreClassifier{}
	c.initPatterns()
	return c
}

// Name returns the classifier name.
func (c *SubjectScoreClassifier) Name() string {
	return "subject"
}

// Stage returns the pipeline stage number.
func (c *SubjectScoreClassifier) Stage() int {
	return 0 // Same stage as RFC and Domain
}

// Classify performs subject pattern-based classification.
// Priority is calculated using pattern scores from priority_score.go
func (c *SubjectScoreClassifier) Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error) {
	if input.Email == nil || input.Email.Subject == "" {
		return nil, nil
	}

	subject := strings.ToLower(input.Email.Subject)

	// Check for critical patterns first (fixed max priority)
	if criticalPriority, isCritical := c.checkCriticalPatterns(subject); isCritical {
		return &ScoreClassifierResult{
			Category:    domain.CategoryWork,
			SubCategory: func() *domain.EmailSubCategory { s := domain.SubCategoryAlert; return &s }(),
			Priority:    domain.Priority(criticalPriority),
			Score:       0.99,
			Source:      "subject:critical",
			Signals:     []string{"subject:critical"},
			LLMUsed:     false,
		}, nil
	}

	// Check each pattern
	for _, p := range c.patterns {
		// Keyword matching (faster)
		if len(p.keywords) > 0 {
			matched := false
			for _, kw := range p.keywords {
				if strings.Contains(subject, kw) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Regex matching (more precise)
		if p.pattern != nil {
			if !p.pattern.MatchString(subject) {
				continue
			}
		}

		// Match found - calculate priority using subject score + category bonus
		subCat := p.subCategory
		subjectScore := getSubjectPatternScore(p.source)
		categoryBonus := getCategoryBonus(p.category)
		priority := CalculatePriority(subjectScore, categoryBonus, 0, 0)

		return &ScoreClassifierResult{
			Category:    p.category,
			SubCategory: &subCat,
			Priority:    domain.Priority(priority),
			Score:       p.score,
			Source:      p.source,
			Signals:     []string{"subject:" + p.source},
			LLMUsed:     false,
		}, nil
	}

	return nil, nil
}

// checkCriticalPatterns checks for server down, crash, security breach patterns
func (c *SubjectScoreClassifier) checkCriticalPatterns(subject string) (float64, bool) {
	criticalPatterns := map[string]float64{
		"is down":             PriorityServerDown,
		"server down":         PriorityServerDown,
		"service down":        PriorityServerDown,
		"crashed":             PriorityServerCrash,
		"crash detected":      PriorityServerCrash,
		"security breach":     PrioritySecurityBreach,
		"data breach":         PriorityDataLoss,
		"unauthorized access": PrioritySecurityBreach,
	}

	for pattern, priority := range criticalPatterns {
		if strings.Contains(subject, pattern) {
			return priority, true
		}
	}
	return 0, false
}

// getSubjectPatternScore returns the priority score for a subject pattern
func getSubjectPatternScore(source string) float64 {
	scores := map[string]float64{
		// CI/CD
		"ci-failed":      SubjectScoreBuildFailed,
		"ci-passed":      SubjectScoreBuildPassed,
		"deploy-failed":  SubjectScoreDeployFailed,
		"deploy-success": SubjectScoreDeploySuccess,
		"test-failed":    SubjectScoreBuildFailed,
		"test-passed":    SubjectScoreBuildPassed,

		// Server/Infra alerts
		"server-down":     SubjectScoreServerDown,
		"server-up":       0.10,
		"crash-alert":     SubjectScoreCrash,
		"error-rate":      0.25,
		"resource-alert":  0.22,
		"disk-alert":      0.22,
		"cert-alert":      0.25,
		"latency-alert":   0.20,
		"backup-failed":   0.28,
		"backup-complete": 0.05,

		// Finance
		"invoice":           SubjectScoreInvoice,
		"receipt":           SubjectScoreReceipt,
		"payment-confirmed": SubjectScorePaymentSuccess,
		"payment-failed":    SubjectScorePaymentFailed,

		// Shopping/Shipping
		"order-confirmed":  SubjectScoreOrder,
		"shipped":          SubjectScoreShipped,
		"out-for-delivery": SubjectScoreShipped,
		"delivered":        SubjectScoreDelivered,

		// Calendar
		"meeting-invite":    SubjectScoreMeetingInvite,
		"meeting-cancelled": SubjectScoreMeetingCancel,
		"meeting-reminder":  0.15,

		// Security
		"password-reset":   0.25,
		"new-signin":       0.22,
		"2fa-code":         0.20,
		"security-warning": SubjectScoreSecurityAlert,

		// Marketing (low)
		"promotion":      SubjectScorePromotion,
		"newsletter-tag": SubjectScoreNewsletter,
		"weekly-digest":  SubjectScoreNewsletter,
		"daily-digest":   SubjectScoreNewsletter,
	}

	if score, ok := scores[source]; ok {
		return score
	}
	return 0.10 // Default
}

func (c *SubjectScoreClassifier) initPatterns() {
	c.patterns = []subjectPattern{
		// =============================================================================
		// Developer / CI-CD Patterns
		// =============================================================================
		{
			keywords:    []string{"build failed", "build failure", "ci failed", "pipeline failed"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "ci-failed",
		},
		{
			keywords:    []string{"build succeeded", "build success", "pipeline passed", "ci passed"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityLow,
			score:       0.88,
			source:      "ci-passed",
		},
		{
			keywords:    []string{"deployment failed", "deploy failed"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "deploy-failed",
		},
		{
			keywords:    []string{"deployment succeeded", "deployed to", "deploy succeeded"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "deploy-success",
		},
		{
			pattern:     regexp.MustCompile(`\[.*\]\s*(pull request|pr|merge request|mr)`),
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "pr-notification",
		},
		{
			keywords:    []string{"review requested", "requested your review"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "review-request",
		},
		{
			keywords:    []string{"security alert", "vulnerability", "dependabot"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "security-alert",
		},

		// === Server/Infra Alert Patterns ===
		{
			keywords:    []string{"is down", "서버 다운", "서비스 중단"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityUrgent,
			score:       0.95,
			source:      "server-down",
		},
		{
			keywords:    []string{"is up", "서버 복구", "서비스 복구"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "server-up",
		},
		{
			keywords:    []string{"crashed", "크래시", "장애 발생"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityUrgent,
			score:       0.95,
			source:      "crash-alert",
		},
		{
			keywords:    []string{"error rate", "에러율", "오류율"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "error-rate",
		},
		{
			keywords:    []string{"high cpu", "high memory", "cpu 사용률", "메모리 사용률"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "resource-alert",
		},
		{
			keywords:    []string{"disk space", "storage full", "디스크 용량", "스토리지 부족"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "disk-alert",
		},
		{
			keywords:    []string{"ssl certificate", "인증서 만료", "certificate expir"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "cert-alert",
		},
		{
			keywords:    []string{"latency alert", "response time", "지연 시간", "응답 시간"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.88,
			source:      "latency-alert",
		},

		// === Approval Workflow Patterns ===
		{
			keywords:    []string{"approved", "승인됨", "승인되었습니다", "승인 완료"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "approved",
		},
		{
			keywords:    []string{"rejected", "거부됨", "거절됨", "반려됨"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "rejected",
		},
		{
			keywords:    []string{"pending approval", "승인 대기", "승인 요청"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "pending-approval",
		},
		{
			keywords:    []string{"needs review", "검토 필요", "리뷰 필요"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "needs-review",
		},

		// === Test Results Patterns ===
		{
			keywords:    []string{"test failed", "tests failed", "테스트 실패"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "test-failed",
		},
		{
			keywords:    []string{"test passed", "tests passed", "all tests pass", "테스트 성공"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityLow,
			score:       0.85,
			source:      "test-passed",
		},
		{
			keywords:    []string{"coverage report", "코드 커버리지"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityLow,
			score:       0.82,
			source:      "coverage-report",
		},

		// === Database Patterns ===
		{
			keywords:    []string{"backup completed", "백업 완료"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityLow,
			score:       0.85,
			source:      "backup-complete",
		},
		{
			keywords:    []string{"backup failed", "백업 실패"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "backup-failed",
		},
		{
			keywords:    []string{"replication lag", "복제 지연"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "replication-lag",
		},
		{
			keywords:    []string{"connection pool", "연결 풀"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.88,
			source:      "connection-pool",
		},

		// =============================================================================
		// Finance Patterns
		// =============================================================================
		{
			keywords:    []string{"invoice", "청구서", "인보이스"},
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryInvoice,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "invoice",
		},
		{
			keywords:    []string{"receipt", "영수증"},
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryReceipt,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "receipt",
		},
		{
			keywords:    []string{"payment received", "payment confirmed", "결제 완료", "결제가 완료"},
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryReceipt,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "payment-confirmed",
		},
		{
			keywords:    []string{"payment failed", "결제 실패", "결제가 실패"},
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryReceipt,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "payment-failed",
		},
		{
			keywords:    []string{"subscription", "구독"},
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryReceipt,
			priority:    domain.PriorityNormal,
			score:       0.85,
			source:      "subscription",
		},

		// =============================================================================
		// Shopping / Order Patterns
		// =============================================================================
		{
			keywords:    []string{"order confirmed", "주문 확인", "주문이 확인"},
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "order-confirmed",
		},
		{
			keywords:    []string{"shipped", "배송 시작", "배송이 시작", "발송되었습니다"},
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "shipped",
		},
		{
			keywords:    []string{"out for delivery", "배송 중", "배달 중"},
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "out-for-delivery",
		},
		{
			keywords:    []string{"delivered", "배송 완료", "배달 완료"},
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "delivered",
		},
		{
			keywords:    []string{"tracking", "운송장", "배송 조회"},
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.85,
			source:      "tracking",
		},

		// =============================================================================
		// Travel Patterns
		// =============================================================================
		{
			keywords:    []string{"flight confirmation", "항공권 확인", "예약 확인"},
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "flight-confirmation",
		},
		{
			keywords:    []string{"boarding pass", "탑승권"},
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "boarding-pass",
		},
		{
			keywords:    []string{"hotel confirmation", "호텔 예약", "숙소 예약"},
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "hotel-confirmation",
		},
		{
			keywords:    []string{"itinerary", "여행 일정"},
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "itinerary",
		},
		{
			keywords:    []string{"check-in reminder", "체크인 안내"},
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "checkin-reminder",
		},

		// =============================================================================
		// Calendar / Meeting Patterns
		// =============================================================================
		{
			keywords:    []string{"meeting invitation", "calendar invitation", "일정 초대", "미팅 초대"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryCalendar,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "meeting-invite",
		},
		{
			keywords:    []string{"meeting cancelled", "meeting canceled", "일정 취소", "미팅 취소"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryCalendar,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "meeting-cancelled",
		},
		{
			keywords:    []string{"meeting reminder", "일정 알림", "미팅 알림"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryCalendar,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "meeting-reminder",
		},
		{
			keywords:    []string{"rescheduled", "일정 변경"},
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryCalendar,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "meeting-rescheduled",
		},

		// =============================================================================
		// Account / Security Patterns
		// =============================================================================
		{
			keywords:    []string{"verify your email", "이메일 인증", "이메일을 인증"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "email-verify",
		},
		{
			keywords:    []string{"password reset", "비밀번호 재설정", "비밀번호 변경"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategorySecurity,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "password-reset",
		},
		{
			keywords:    []string{"new sign-in", "new login", "새로운 로그인", "새 기기에서 로그인"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategorySecurity,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "new-signin",
		},
		{
			keywords:    []string{"two-factor", "2fa", "인증 코드", "verification code"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategorySecurity,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "2fa-code",
		},
		{
			keywords:    []string{"suspicious activity", "의심스러운 활동", "보안 경고"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategorySecurity,
			priority:    domain.PriorityUrgent,
			score:       0.95,
			source:      "security-warning",
		},
		{
			keywords:    []string{"account locked", "계정이 잠겼", "계정 잠금"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategorySecurity,
			priority:    domain.PriorityUrgent,
			score:       0.95,
			source:      "account-locked",
		},

		// =============================================================================
		// Social Patterns
		// =============================================================================
		{
			keywords:    []string{"mentioned you", "님이 회원님을 언급", "님이 멘션"},
			category:    domain.CategorySocial,
			subCategory: domain.SubCategoryComment,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "mention",
		},
		{
			keywords:    []string{"commented on", "님이 댓글", "새 댓글"},
			category:    domain.CategorySocial,
			subCategory: domain.SubCategoryComment,
			priority:    domain.PriorityNormal,
			score:       0.85,
			source:      "comment",
		},
		{
			keywords:    []string{"started following", "님이 팔로우", "새로운 팔로워"},
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.85,
			source:      "new-follower",
		},
		{
			keywords:    []string{"liked your", "님이 좋아요"},
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLowest,
			score:       0.82,
			source:      "like",
		},

		// =============================================================================
		// Newsletter / Marketing Patterns
		// =============================================================================
		{
			pattern:     regexp.MustCompile(`\[\s*newsletter\s*\]|뉴스레터`),
			category:    domain.CategoryNewsletter,
			subCategory: domain.SubCategoryNewsletter,
			priority:    domain.PriorityLow,
			score:       0.88,
			source:      "newsletter-tag",
		},
		{
			keywords:    []string{"weekly digest", "주간 소식", "weekly update"},
			category:    domain.CategoryNewsletter,
			subCategory: domain.SubCategoryNewsletter,
			priority:    domain.PriorityLow,
			score:       0.88,
			source:      "weekly-digest",
		},
		{
			keywords:    []string{"daily digest", "일일 소식", "daily update"},
			category:    domain.CategoryNewsletter,
			subCategory: domain.SubCategoryNewsletter,
			priority:    domain.PriorityLow,
			score:       0.88,
			source:      "daily-digest",
		},
		{
			keywords:    []string{"% off", "% 할인", "sale", "세일", "discount", "할인"},
			category:    domain.CategoryMarketing,
			subCategory: domain.SubCategoryDeal,
			priority:    domain.PriorityLow,
			score:       0.85,
			source:      "promotion",
		},
		{
			keywords:    []string{"limited time", "한정 기간", "오늘만", "today only"},
			category:    domain.CategoryMarketing,
			subCategory: domain.SubCategoryDeal,
			priority:    domain.PriorityLow,
			score:       0.85,
			source:      "limited-offer",
		},
		{
			keywords:    []string{"free shipping", "무료 배송"},
			category:    domain.CategoryMarketing,
			subCategory: domain.SubCategoryDeal,
			priority:    domain.PriorityLow,
			score:       0.82,
			source:      "free-shipping",
		},

		// =============================================================================
		// Alert / Notification Patterns
		// =============================================================================
		{
			keywords:    []string{"alert:", "[alert]", "경고:", "[경고]"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.88,
			source:      "alert-tag",
		},
		{
			keywords:    []string{"action required", "조치 필요", "즉시 확인"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.90,
			source:      "action-required",
		},
		{
			keywords:    []string{"urgent:", "[urgent]", "긴급:", "[긴급]"},
			category:    domain.CategoryNotification,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityUrgent,
			score:       0.92,
			source:      "urgent-tag",
		},
	}
}
