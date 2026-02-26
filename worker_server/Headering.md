# Email Header Classification Guide

> 이메일 헤더 기반 분류 시스템 설계를 위한 연구 문서
> 
> Last Updated: 2025-01-18

---

## 목차

1. [RFC 표준 헤더](#1-rfc-표준-헤더)
2. [ESP (Email Service Provider) 헤더](#2-esp-email-service-provider-헤더)
3. [개발자 서비스 헤더](#3-개발자-서비스-헤더)
4. [클라우드/인프라 서비스](#4-클라우드인프라-서비스-헤더)
5. [모니터링/로깅 서비스](#5-모니터링로깅-서비스)
6. [커뮤니케이션 서비스](#6-커뮤니케이션-서비스)
7. [금융/결제 서비스](#7-금융결제-서비스)
8. [쇼핑/이커머스 서비스](#8-쇼핑이커머스-서비스)
9. [소셜 미디어](#9-소셜-미디어)
10. [한국 서비스](#10-한국-서비스)
11. [Subject 패턴](#11-subject-패턴)
12. [분류 우선순위 매트릭스](#12-분류-우선순위-매트릭스)
13. [구현 계획](#13-구현-계획)

---

## 1. RFC 표준 헤더

### 1.1 List-* 헤더 (RFC 2369, RFC 8058)

| 헤더 | 용도 | 분류 | 신뢰도 |
|------|------|------|--------|
| `List-Unsubscribe` | 구독 취소 링크 | Newsletter/Marketing | 0.95 |
| `List-Unsubscribe-Post` | One-click 구독 취소 (RFC 8058) | Newsletter/Marketing | 0.98 |
| `List-ID` | 메일링 리스트 식별 | Newsletter | 0.90 |
| `List-Post` | 리스트에 글 작성 | Newsletter | 0.85 |
| `List-Help` | 도움말 링크 | Newsletter | 0.80 |
| `List-Subscribe` | 구독 링크 | Newsletter | 0.85 |
| `List-Archive` | 아카이브 링크 | Newsletter | 0.80 |

**참고**: Gmail, Yahoo 등 주요 이메일 제공자는 대량 발송자에게 List-Unsubscribe 헤더를 필수로 요구 ([Google Email Guidelines](https://support.google.com/a/answer/81126))

### 1.2 Precedence 헤더

| 값 | 의미 | 분류 | 우선순위 | 신뢰도 |
|----|------|------|----------|--------|
| `bulk` | 대량 발송 | Marketing | Lowest | 0.90 |
| `list` | 메일링 리스트 | Newsletter | Low | 0.85 |
| `junk` | 정크 메일 | Spam | Lowest | 0.85 |

### 1.3 Auto-Submitted 헤더 (RFC 3834)

| 값 | 의미 | 분류 | 신뢰도 |
|----|------|------|--------|
| `no` | 사람이 보냄 | - (패스) | - |
| `auto-generated` | 자동 생성 | Notification | 0.92 |
| `auto-replied` | 자동 응답 | Notification | 0.92 |
| `auto-notified` | 자동 알림 | Notification | 0.92 |

### 1.4 X-Auto-Response-Suppress (Microsoft)

자동 응답 억제 헤더 - 이 헤더가 있으면 자동 생성 메일임을 의미

| 값 | 의미 | 분류 | 신뢰도 |
|----|------|------|--------|
| `OOF` | Out of Office 억제 | Notification | 0.88 |
| `DR` | Delivery Report 억제 | Notification | 0.88 |
| `RN` | Read Notification 억제 | Notification | 0.88 |
| `NRN` | Non-Read Notification 억제 | Notification | 0.88 |
| `AutoReply` | 자동 응답 억제 | Notification | 0.88 |
| `All` | 모든 자동 응답 억제 | Notification | 0.90 |

### 1.5 기타 RFC 헤더

| 헤더 | 용도 | 분류 힌트 |
|------|------|----------|
| `Reply-To` | 답장 주소 | noreply@ → Notification |
| `X-Priority` | 우선순위 (1=High, 5=Low) | 스팸 지표 (과도 사용 시) |
| `X-Mailer` | 발송 소프트웨어 | ESP 식별 |
| `Feedback-ID` | Gmail 대량 발송자 ID | Marketing (0.80) |
| `Message-ID` | 메시지 고유 ID | 도메인 추출 가능 |

---

## 2. ESP (Email Service Provider) 헤더

ESP 헤더가 있으면 마케팅/트랜잭션 이메일일 가능성이 높음

### 2.1 SendGrid (Twilio)

| 헤더 | 용도 | 신뢰도 |
|------|------|--------|
| `X-SG-EID` | SendGrid Event ID | 0.88 |
| `X-SG-ID` | SendGrid Message ID | 0.88 |
| `X-SMTPAPI` | SendGrid API 설정 (JSON) | 0.90 |

### 2.2 Mailchimp / Mandrill

| 헤더 | 용도 | 신뢰도 |
|------|------|--------|
| `X-MC-User` | Mailchimp User ID | 0.90 |
| `X-MC-Metadata` | 캠페인 메타데이터 | 0.90 |
| `X-Mailchimp-*` | Mailchimp 식별 | 0.90 |
| `X-Mandrill-User` | Mandrill User | 0.88 |

### 2.3 Amazon SES

| 헤더 | 용도 | 신뢰도 |
|------|------|--------|
| `X-SES-Outgoing` | SES 발송 식별 | 0.85 |
| `X-SES-MESSAGE-TAGS` | 메시지 태그 | 0.85 |
| `X-SES-CONFIGURATION-SET` | 설정 세트 | 0.85 |

**참고**: SES는 트랜잭션/마케팅 모두 사용되므로 신뢰도가 낮음

### 2.4 Mailgun

| 헤더 | 용도 | 신뢰도 |
|------|------|--------|
| `X-Mailgun-Sid` | Mailgun Session ID | 0.85 |
| `X-Mailgun-Tag` | 메시지 태그 | 0.85 |
| `X-Mailgun-Variables` | 커스텀 변수 | 0.85 |
| `X-Mailgun-Dkim` | DKIM 서명 | 0.85 |

### 2.5 Postmark

| 헤더 | 용도 | 신뢰도 |
|------|------|--------|
| `X-PM-Message-Id` | Postmark Message ID | 0.85 |
| `X-PM-Tag` | 메시지 태그 | 0.85 |

**참고**: Postmark는 트랜잭션 전문이므로 Marketing보다 Notification 가능성

### 2.6 기타 ESP

| ESP | 헤더 패턴 | 주요 용도 | 신뢰도 |
|-----|----------|----------|--------|
| HubSpot | `X-HubSpot-*` | Marketing | 0.90 |
| Salesforce | `X-SFDC-*` | Marketing/CRM | 0.88 |
| Intercom | `X-Intercom-*` | Support/Marketing | 0.85 |
| Customer.io | `X-Customerio-*` | Marketing | 0.88 |
| Braze | `X-Braze-*` | Marketing | 0.88 |
| Klaviyo | `X-Klaviyo-*` | Marketing | 0.88 |
| ActiveCampaign | `X-AC-*` | Marketing | 0.88 |
| ConvertKit | `X-ConvertKit-*` | Newsletter | 0.88 |

---

## 3. 개발자 서비스 헤더

### 3.1 GitHub ⭐

**도메인**: `github.com`, `notifications@github.com`

**X-GitHub-Reason 헤더** (가장 중요한 분류 기준):

| 헤더 값 | 설명 | 분류 | 우선순위 | 신뢰도 |
|---------|------|------|----------|--------|
| `review_requested` | PR 리뷰 요청 | Developer/CodeReview | High | 0.98 |
| `author` | 내가 만든 이슈/PR 업데이트 | Developer/CodeReview | Normal | 0.98 |
| `comment` | 코멘트 알림 | Developer/CodeReview | Normal | 0.98 |
| `mention` | @멘션 | Developer/CodeReview | High | 0.98 |
| `team_mention` | 팀 멘션 | Developer/CodeReview | High | 0.98 |
| `assign` | 이슈/PR 할당됨 | Developer/CodeReview | High | 0.98 |
| `security_alert` | 보안 취약점 알림 | Developer/Security | **Urgent** | 0.99 |
| `ci_activity` | GitHub Actions 완료 | Developer/CI | Low | 0.98 |
| `push` | 푸시 알림 | Developer/CI | Low | 0.98 |
| `state_change` | 상태 변경 (open/close/merge) | Developer/CodeReview | Normal | 0.98 |
| `subscribed` | 구독 중인 저장소 | Developer/Notification | Low | 0.98 |
| `manual` | 수동 구독 | Developer/Notification | Low | 0.98 |
| `your_activity` | 내 활동 알림 | Developer/Notification | Lowest | 0.98 |

**X-GitHub-Severity 헤더** (Dependabot 보안 알림):

| 헤더 값 | 설명 | 우선순위 | 신뢰도 |
|---------|------|----------|--------|
| `critical` | 치명적 취약점 | **Urgent** | 0.99 |
| `high` | 높은 위험 | High | 0.99 |
| `moderate` | 보통 위험 | Normal | 0.99 |
| `low` | 낮은 위험 | Low | 0.99 |

**추가 GitHub 헤더**:

| 헤더 | 용도 |
|------|------|
| `X-GitHub-Recipient` | 수신자 사용자명 |
| `X-GitHub-Recipient-Address` | 수신자 이메일 |
| `X-GitHub-Sender` | 발신자 사용자명 |

**CC 주소 패턴** (헤더 미지원 클라이언트용):
```
review_requested@noreply.github.com
author@noreply.github.com
mention@noreply.github.com
security_alert@noreply.github.com
ci_activity@noreply.github.com
```

**List-ID 패턴**: `OWNER/REPO <REPO.OWNER.github.com>`

### 3.2 GitLab

**도메인**: `gitlab.com`, `*.gitlab.com`

| 헤더 | 용도 | 분류 | 신뢰도 |
|------|------|------|--------|
| `X-GitLab-Project` | 프로젝트 식별 | Developer | 0.95 |
| `X-GitLab-Project-Id` | 프로젝트 ID | Developer | 0.95 |
| `X-GitLab-Project-Path` | 프로젝트 경로 | Developer | 0.95 |
| `X-GitLab-Pipeline-Id` | 파이프라인 ID | Developer/CI | 0.95 |
| `X-GitLab-NotificationReason` | 알림 이유 | Developer | 0.95 |

### 3.3 Bitbucket (Atlassian)

**도메인**: `bitbucket.org`, `*.atlassian.com`

| 헤더 | 용도 | 분류 | 신뢰도 |
|------|------|------|--------|
| `X-Bitbucket-*` | Bitbucket 알림 | Developer | 0.90 |

### 3.4 Jira (Atlassian)

**도메인**: `*.atlassian.net`, `*.jira.com`

| 헤더 | 용도 | 분류 | 신뢰도 |
|------|------|------|--------|
| `X-JIRA-FingerPrint` | Jira 인스턴스 식별 | Work/Task | 0.95 |

**조합 패턴**:
- `X-JIRA-FingerPrint` + `Auto-Submitted: auto-generated` → Work/Task

### 3.5 Linear

**도메인**: `linear.app`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `notifications@linear.app` | Work/Task | 0.95 |

**Webhook 헤더** (참고용):
- `Linear-Delivery`: 배송 UUID
- `Linear-Event`: 이벤트 타입 (Issue, Comment 등)

### 3.6 Notion

**도메인**: `notion.so`, `mail.notion.so`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `notify@mail.notion.so` | Work/Notification | 0.90 |
| `*@notion.so` | Work/Notification | 0.85 |

### 3.7 Figma

**도메인**: `figma.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@figma.com` | Work/Design | 0.90 |

**참고**: Figma는 모든 공식 이메일을 `@figma.com` 도메인에서 발송

### 3.8 Asana

**도메인**: `asana.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@asana.com` | Work/Task | 0.90 |

### 3.9 Trello

**도메인**: `trello.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@trello.com` | Work/Task | 0.90 |

---

## 4. 클라우드/인프라 서비스 헤더

### 4.1 Vercel

**도메인**: `vercel.com`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `notifications@vercel.com` | Developer/CI | Normal | 0.95 |
| `security@vercel.com` | Developer/Security | High | 0.98 |

**X-Vercel 헤더** (HTTP 응답용, 이메일에는 미포함):
- `X-Vercel-Id`, `X-Vercel-Cache`, `X-Vercel-Deployment-Url`

**Subject 패턴으로 분류**:
| Subject 패턴 | 분류 | 우선순위 |
|-------------|------|----------|
| `*Deployment*succeeded*` | Developer/CI | Low |
| `*Deployment*failed*` | Developer/CI | High |
| `*Build*failed*` | Developer/CI | High |

### 4.2 Railway

**도메인**: `railway.app`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `notifications@railway.app` | Developer/CI | Normal | 0.95 |

**Subject 패턴으로 분류**:
| Subject 패턴 | 분류 | 우선순위 |
|-------------|------|----------|
| `*Deploy succeeded*` | Developer/CI | Low |
| `*Deploy failed*` | Developer/CI | High |
| `*Service crashed*` | Developer/Incident | **Urgent** |
| `*Service restarted*` | Developer/Incident | Normal |

### 4.3 Netlify

**도메인**: `netlify.com`, `netlify.app`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `team@netlify.com` | Developer/CI | 0.90 |
| `no-reply@netlify.com` | Developer/CI | 0.90 |

### 4.4 Render

**도메인**: `render.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@render.com` | Developer/CI | 0.90 |

### 4.5 Fly.io

**도메인**: `fly.io`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@fly.io` | Developer/CI | 0.90 |

### 4.6 AWS (Amazon Web Services)

**도메인**: `*.amazonaws.com`, `aws.amazon.com`, `amazon.com`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `no-reply@sns.amazonaws.com` | Developer/Notification | Normal | 0.90 |
| `no-reply@cloudwatch.amazonaws.com` | Developer/Monitoring | Normal | 0.90 |
| `no-reply@ses.amazonaws.com` | Developer/Notification | Normal | 0.85 |
| `billing@aws.amazon.com` | Finance/Invoice | Normal | 0.95 |
| `aws-security@amazon.com` | Developer/Security | High | 0.98 |
| `account-update@amazon.com` | Finance/Account | Normal | 0.90 |

### 4.7 Google Cloud Platform (GCP)

**도메인**: `*.google.com`, `cloud.google.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `cloudnotifications-noreply@google.com` | Developer/Notification | 0.90 |
| `billing-noreply@google.com` | Finance/Invoice | 0.95 |
| `noreply-cloud@google.com` | Developer/Notification | 0.90 |

### 4.8 Microsoft Azure

**도메인**: `*.microsoft.com`, `azure.com`, `azure.microsoft.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `azureadvisor-noreply@microsoft.com` | Developer/Notification | 0.90 |
| `azure-noreply@microsoft.com` | Developer/Notification | 0.90 |
| `azuresupport-noreply@microsoft.com` | Developer/Support | 0.90 |

### 4.9 DigitalOcean

**도메인**: `digitalocean.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `no-reply@digitalocean.com` | Developer/Notification | 0.90 |

### 4.10 Heroku

**도메인**: `heroku.com`, `salesforce.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `bot@heroku.com` | Developer/CI | 0.90 |
| `noreply@heroku.com` | Developer/Notification | 0.90 |

### 4.11 Cloudflare

**도메인**: `cloudflare.com`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `noreply@cloudflare.com` | Developer/Notification | Normal | 0.90 |
| `security@cloudflare.com` | Developer/Security | High | 0.95 |

---

## 5. 모니터링/로깅 서비스

### 5.1 Sentry

**도메인**: `sentry.io`, `getsentry.com`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `noreply@sentry.io` | Developer/Incident | High | 0.95 |
| `noreply@md.getsentry.com` | Developer/Incident | High | 0.95 |

**Subject 패턴으로 분류**:
| Subject 패턴 | 분류 | 우선순위 |
|-------------|------|----------|
| `*Error*`, `*Exception*` | Developer/Incident | High |
| `*resolved*` | Developer/Incident | Low |
| `*regression*` | Developer/Incident | High |

### 5.2 Datadog

**도메인**: `datadoghq.com`, `dtdg.co`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `no-reply@dtdg.co` | Developer/Monitoring | 0.95 |
| `alerts@datadoghq.com` | Developer/Monitoring | 0.95 |

### 5.3 PagerDuty ⚠️

**도메인**: `pagerduty.com`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `*@pagerduty.com` | Developer/Incident | **Urgent** | 0.98 |

**참고**: PagerDuty 이메일은 항상 Urgent로 처리

### 5.4 New Relic

**도메인**: `newrelic.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `noreply@newrelic.com` | Developer/Monitoring | 0.90 |
| `alerts@newrelic.com` | Developer/Monitoring | 0.95 |

### 5.5 Grafana

**도메인**: `grafana.com`, `grafana.net`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@grafana.com` | Developer/Monitoring | 0.90 |
| `*@grafana.net` | Developer/Monitoring | 0.90 |

### 5.6 Uptime Robot

**도메인**: `uptimerobot.com`

| Subject 패턴 | 분류 | 우선순위 | 신뢰도 |
|-------------|------|----------|--------|
| `*is DOWN*` | Developer/Incident | **Urgent** | 0.98 |
| `*is UP*` | Developer/Incident | Low | 0.95 |

### 5.7 Pingdom

**도메인**: `pingdom.com`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `*@pingdom.com` | Developer/Monitoring | High | 0.90 |

### 5.8 Opsgenie (Atlassian)

**도메인**: `opsgenie.com`, `atlassian.com`

| From 패턴 | 분류 | 우선순위 | 신뢰도 |
|-----------|------|----------|--------|
| `*@opsgenie.com` | Developer/Incident | **Urgent** | 0.98 |

### 5.9 CircleCI

**도메인**: `circleci.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@circleci.com` | Developer/CI | 0.90 |

### 5.10 Travis CI

**도메인**: `travis-ci.com`, `travis-ci.org`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@travis-ci.com` | Developer/CI | 0.90 |
| `*@travis-ci.org` | Developer/CI | 0.90 |

### 5.11 Jenkins

**참고**: Jenkins는 자체 호스팅이므로 도메인이 다양함. Subject 패턴으로 식별:

| Subject 패턴 | 분류 | 우선순위 |
|-------------|------|----------|
| `*Build*failed*` | Developer/CI | High |
| `*Build*succeeded*` | Developer/CI | Low |
| `*Build*unstable*` | Developer/CI | Normal |

---

## 6. 커뮤니케이션 서비스

### 6.1 Slack

**도메인**: `slack.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `notification@slack.com` | Work/Notification | 0.90 |
| `no-reply@slack.com` | Work/Notification | 0.90 |
| `feedback@slack.com` | Marketing | 0.80 |

### 6.2 Discord

**도메인**: `discord.com`, `discordapp.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `noreply@discord.com` | Social/Notification | 0.90 |
| `noreply@discordapp.com` | Social/Notification | 0.90 |

### 6.3 Microsoft Teams

**도메인**: `teams.microsoft.com`, `microsoft.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `noreply@email.teams.microsoft.com` | Work/Notification | 0.95 |

### 6.4 Zoom

**도메인**: `zoom.us`, `zoom.com`

| From 패턴 | Subject 패턴 | 분류 | 신뢰도 |
|-----------|-------------|------|--------|
| `no-reply@zoom.us` | `*meeting*`, `*invitation*` | Work/Calendar | 0.90 |
| `no-reply@zoom.us` | `*recording*` | Work/Notification | 0.85 |
| `no-reply@zoom.us` | - | Work/Notification | 0.80 |

### 6.5 Google Meet / Calendar

**도메인**: `google.com`, `calendar.google.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `calendar-notification@google.com` | Work/Calendar | 0.95 |
| `noreply-calendar-sync@google.com` | Work/Calendar | 0.90 |

---

## 7. 금융/결제 서비스

### 7.1 Stripe

**도메인**: `stripe.com`

| From 패턴 | 분류 | 서브카테고리 | 신뢰도 |
|-----------|------|-------------|--------|
| `receipts@stripe.com` | Finance | Receipt | 0.98 |
| `notifications@stripe.com` | Finance | Payment | 0.95 |
| `billing@stripe.com` | Finance | Invoice | 0.95 |

**Subject 패턴**:
| Subject 패턴 | 서브카테고리 |
|-------------|-------------|
| `*payment*` | Payment |
| `*invoice*` | Invoice |
| `*receipt*` | Receipt |

### 7.2 PayPal

**도메인**: `paypal.com`, `paypal.co.kr`

| From 패턴 | 분류 | 서브카테고리 | 신뢰도 |
|-----------|------|-------------|--------|
| `service@paypal.com` | Finance | Payment | 0.95 |
| `member@paypal.com` | Finance | Account | 0.90 |

### 7.3 Wise (TransferWise)

**도메인**: `wise.com`, `transferwise.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@wise.com` | Finance | 0.90 |
| `*@transferwise.com` | Finance | 0.90 |

### 7.4 Paddle

**도메인**: `paddle.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@paddle.com` | Finance | 0.90 |

### 7.5 Gumroad

**도메인**: `gumroad.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@gumroad.com` | Finance | 0.90 |

### 7.6 Plaid

**도메인**: `plaid.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@plaid.com` | Finance | 0.90 |

### 7.7 Brex

**도메인**: `brex.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@brex.com` | Finance | 0.95 |
| `notifications@brex.com` | Finance | 0.95 |

### 7.8 Ramp

**도메인**: `ramp.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@ramp.com` | Finance | 0.95 |

### 7.9 Mercury

**도메인**: `mercury.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@mercury.com` | Finance | 0.95 |

### 7.10 Revolut

**도메인**: `revolut.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@revolut.com` | Finance | 0.90 |

### 7.11 N26

**도메인**: `n26.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@n26.com` | Finance | 0.90 |

### 7.12 Square / Cash App

**도메인**: `squareup.com`, `square.com`, `cash.app`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@squareup.com` | Finance | 0.90 |
| `*@square.com` | Finance | 0.90 |
| `*@cash.app` | Finance | 0.90 |

### 7.13 Venmo

**도메인**: `venmo.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@venmo.com` | Finance | 0.90 |

### 7.14 Coinbase

**도메인**: `coinbase.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@coinbase.com` | Finance | 0.90 |
| `no-reply@coinbase.com` | Finance | 0.90 |

### 7.15 Binance

**도메인**: `binance.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@binance.com` | Finance | 0.85 |
| `do-not-reply@binance.com` | Finance | 0.85 |

---

## 8. 쇼핑/이커머스 서비스

### 8.1 Amazon

**도메인**: `amazon.com`, `amazon.co.kr`, `amazon.co.jp`, `amazon.de`, `amazon.co.uk`

| From 패턴 | Subject 패턴 | 분류 | 서브카테고리 | 신뢰도 |
|-----------|-------------|------|-------------|--------|
| `*@amazon.com` | `*shipped*`, `*배송*` | Shopping | Shipping | 0.95 |
| `*@amazon.com` | `*delivered*`, `*배달*` | Shopping | Shipping | 0.95 |
| `*@amazon.com` | `*order*`, `*주문*` | Shopping | Order | 0.90 |
| `*@amazon.com` | - | Shopping | - | 0.80 |

### 8.2 eBay

**도메인**: `ebay.com`, `ebay.co.uk`, `ebay.de`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@ebay.com` | Shopping | 0.85 |
| `*@ebay.co.uk` | Shopping | 0.85 |

### 8.3 AliExpress / Alibaba

**도메인**: `aliexpress.com`, `alibaba.com`, `1688.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@aliexpress.com` | Shopping | 0.85 |
| `*@alibaba.com` | Shopping | 0.85 |

### 8.4 Shopify (판매자용)

**도메인**: `shopify.com`, `myshopify.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@shopify.com` | Work/Notification | 0.85 |
| `*@myshopify.com` | Shopping | 0.80 |

### 8.5 Etsy

**도메인**: `etsy.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@etsy.com` | Shopping | 0.85 |
| `*@mail.etsy.com` | Shopping | 0.85 |

### 8.6 Walmart

**도메인**: `walmart.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@walmart.com` | Shopping | 0.85 |

### 8.7 Target

**도메인**: `target.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@target.com` | Shopping | 0.85 |

### 8.8 Best Buy

**도메인**: `bestbuy.com`

| From 패턴 | 분류 | 신뢰도 |
|-----------|------|--------|
| `*@bestbuy.com` | Shopping | 0.85 |

---

## 9. 소셜 미디어

| 서비스 | 도메인 | 분류 | 신뢰도 |
|--------|--------|------|--------|
| Twitter/X | `twitter.com`, `x.com` | Social | 0.90 |
| LinkedIn | `linkedin.com`, `e.linkedin.com` | Social | 0.90 |
| Facebook | `facebookmail.com` | Social | 0.90 |
| Instagram | `mail.instagram.com` | Social | 0.90 |
| TikTok | `tiktok.com` | Social | 0.85 |
| YouTube | `youtube.com` | Social | 0.85 |
| Reddit | `reddit.com`, `redditmail.com` | Social | 0.85 |

---

## 10. 한국 서비스

### 10.1 금융/결제

| 서비스 | 도메인 | From 패턴 | 분류 | 신뢰도 |
|--------|--------|-----------|------|--------|
| 토스 | `toss.im` | `*@toss.im` | Finance | 0.95 |
| 카카오페이 | `kakaopay.com` | `*@kakaopay.com` | Finance | 0.95 |
| 네이버페이 | `naverpay.com`, `naver.com` | `*@naverpay.com` | Finance | 0.90 |

**은행**:
| 서비스 | 도메인 | 분류 | 신뢰도 |
|--------|--------|------|--------|
| 신한은행 | `shinhan.com` | Finance | 0.90 |
| 우리은행 | `wooribank.com` | Finance | 0.90 |
| KB국민은행 | `kbstar.com` | Finance | 0.90 |
| 하나은행 | `hanabank.com` | Finance | 0.90 |
| NH농협 | `nonghyup.com` | Finance | 0.90 |

### 10.2 쇼핑/이커머스

| 서비스 | 도메인 | From 패턴 | 분류 | 신뢰도 |
|--------|--------|-----------|------|--------|
| 쿠팡 | `coupang.com` | `no_reply@coupang.com` | Shopping | 0.95 |
| 네이버쇼핑 | `shop.naver.com`, `naver.com` | `*@naver.com` | Shopping | 0.85 |
| 11번가 | `11st.co.kr` | `*@11st.co.kr` | Shopping | 0.90 |
| G마켓 | `gmarket.co.kr` | `*@gmarket.co.kr` | Shopping | 0.90 |
| 옥션 | `auction.co.kr` | `*@auction.co.kr` | Shopping | 0.90 |
| 배달의민족 | `baemin.com` | `*@baemin.com` | Shopping | 0.90 |
| 요기요 | `yogiyo.co.kr` | `*@yogiyo.co.kr` | Shopping | 0.90 |
| 마켓컬리 | `kurly.com` | `*@kurly.com` | Shopping | 0.90 |

**주의**: 쿠팡 공식 이메일은 `no_reply@coupang.com` (언더바 있음). `noreply@coupang.com` (언더바 없음)은 피싱 가능성

### 10.3 배송

| 서비스 | 도메인 | 분류 | 서브카테고리 | 신뢰도 |
|--------|--------|------|-------------|--------|
| CJ대한통운 | `cjlogistics.com` | Shopping | Shipping | 0.90 |
| 한진택배 | `hanjin.co.kr` | Shopping | Shipping | 0.90 |
| 롯데택배 | `lotteglogis.com` | Shopping | Shipping | 0.90 |
| 로젠택배 | `ilogen.com` | Shopping | Shipping | 0.90 |

### 10.4 포털/서비스

| 서비스 | 도메인 | 분류 | 신뢰도 |
|--------|--------|------|--------|
| 네이버 | `naver.com`, `mail.naver.com` | Notification | 0.80 |
| 카카오 | `kakao.com`, `kakaomail.com` | Notification | 0.80 |
| 다음 | `daum.net`, `hanmail.net` | Notification | 0.80 |

### 10.5 여행

| 서비스 | 도메인 | 분류 | 신뢰도 |
|--------|--------|------|--------|
| 야놀자 | `yanolja.com` | Travel | 0.90 |
| 여기어때 | `goodchoice.kr` | Travel | 0.90 |
| 인터파크투어 | `interpark.com` | Travel | 0.85 |

---

## 11. Subject 패턴

Subject 라인으로 이메일 유형을 추가 분류

### 11.1 개발자/CI-CD 패턴

| 패턴 | 분류 | 우선순위 |
|------|------|----------|
| `*build*failed*`, `*빌드*실패*` | Developer/CI | High |
| `*build*succeeded*`, `*빌드*성공*` | Developer/CI | Low |
| `*deploy*failed*`, `*배포*실패*` | Developer/CI | High |
| `*deploy*succeeded*`, `*배포*성공*` | Developer/CI | Low |
| `*pipeline*failed*` | Developer/CI | High |
| `*test*failed*` | Developer/CI | High |

### 11.2 보안 패턴

| 패턴 | 분류 | 우선순위 |
|------|------|----------|
| `*security*alert*`, `*보안*알림*` | Developer/Security | **Urgent** |
| `*vulnerability*`, `*취약점*` | Developer/Security | High |
| `*suspicious*`, `*의심*` | Developer/Security | High |
| `*2FA*`, `*two-factor*`, `*인증*` | Security | High |
| `*password*reset*`, `*비밀번호*재설정*` | Security | High |
| `*unauthorized*`, `*비인가*` | Security | **Urgent** |

### 11.3 장애/모니터링 패턴

| 패턴 | 분류 | 우선순위 |
|------|------|----------|
| `*is DOWN*`, `*다운*` | Developer/Incident | **Urgent** |
| `*is UP*`, `*복구*` | Developer/Incident | Low |
| `*error*`, `*오류*`, `*에러*` | Developer/Incident | High |
| `*exception*`, `*예외*` | Developer/Incident | High |
| `*crashed*`, `*크래시*` | Developer/Incident | **Urgent** |
| `*timeout*`, `*타임아웃*` | Developer/Incident | High |
| `*alert*`, `*경고*` | Developer/Monitoring | Normal |

### 11.4 코드 리뷰 패턴

| 패턴 | 분류 | 우선순위 |
|------|------|----------|
| `*review requested*`, `*리뷰 요청*` | Developer/CodeReview | High |
| `*approved*`, `*승인*` | Developer/CodeReview | Normal |
| `*changes requested*`, `*수정 요청*` | Developer/CodeReview | High |
| `*merged*`, `*머지*` | Developer/CodeReview | Low |

### 11.5 쇼핑/이커머스 패턴

| 패턴 | 분류 | 서브카테고리 |
|------|------|-------------|
| `*order*confirmed*`, `*주문*확인*` | Shopping | Order |
| `*shipped*`, `*발송*`, `*출고*` | Shopping | Shipping |
| `*delivered*`, `*배달*완료*` | Shopping | Shipping |
| `*out for delivery*`, `*배송*중*` | Shopping | Shipping |
| `*receipt*`, `*영수증*` | Finance | Receipt |
| `*invoice*`, `*청구서*` | Finance | Invoice |

### 11.6 일정/미팅 패턴

| 패턴 | 분류 |
|------|------|
| `*meeting*invitation*`, `*회의*초대*` | Work/Calendar |
| `*calendar*event*`, `*일정*` | Work/Calendar |
| `*reminder*`, `*리마인더*` | Work/Calendar |
| `*rescheduled*`, `*일정*변경*` | Work/Calendar |
| `*cancelled*`, `*취소*` | Work/Calendar |

---

## 12. 분류 우선순위 매트릭스

### 12.1 카테고리별 기본 우선순위

| 카테고리 | 기본 우선순위 | Inbox 포함 | 설명 |
|----------|--------------|------------|------|
| Primary | Normal | ✅ Yes | 중요 개인 메일 |
| Work | Normal | ✅ Yes | 업무 관련 |
| Developer | Normal | ✅ Yes | 개발 도구 알림 |
| Personal | Normal | ✅ Yes | 친구/가족 |
| Finance | Normal | ⚡ 선택 | 금융 |
| Travel | Normal | ⚡ 선택 | 여행 |
| Shopping | Low | ❌ No | 쇼핑 |
| Newsletter | Low | ❌ No | 뉴스레터 |
| Notification | Low | ❌ No | 일반 알림 |
| Marketing | Lowest | ❌ No | 마케팅 |
| Social | Low | ❌ No | SNS |
| Spam | Lowest | ❌ No | 스팸 |
| Other | Normal | ❌ No | 미분류 |

### 12.2 서브카테고리별 우선순위 조정

| 서브카테고리 | 우선순위 | 우선순위 조정 |
|-------------|----------|--------------|
| Security | **Urgent** | +3 (항상 최우선) |
| Incident | **Urgent** | +3 |
| CodeReview | High | +2 |
| Task | Normal | 0 |
| Invoice | Normal | 0 |
| Order | Normal | 0 |
| CI | Low | -1 |
| Shipping | Low | -1 |
| Receipt | Low | -1 |
| Monitoring | Low | -1 |
| Release | Low | -1 |

### 12.3 분류 신호 처리 우선순위

RFC 분류기에서 신호 처리 순서:

```
우선순위 1 (Urgent): 보안 관련
├── X-GitHub-Severity: critical/high
├── Subject: *security*alert*, *vulnerability*
├── From: security@*, aws-security@*
└── PagerDuty, Opsgenie 도메인

우선순위 2 (High): 서비스별 커스텀 헤더
├── X-GitHub-Reason (값에 따라 분류)
├── X-GitLab-* 헤더
└── X-JIRA-FingerPrint

우선순위 3 (Normal): RFC 표준 헤더
├── List-Unsubscribe + List-Unsubscribe-Post → Newsletter (0.98)
├── Auto-Submitted: auto-generated → Notification (0.92)
├── Precedence: bulk → Marketing (0.90)
└── List-ID → Newsletter (0.90)

우선순위 4 (Normal): ESP 헤더
├── X-SG-*, X-MC-*, X-Mailgun-* → Marketing (0.88)
└── Feedback-ID → Marketing (0.80)

우선순위 5 (Low): 패턴 기반
├── No-Reply 패턴 → Notification (0.70-0.85)
├── 도메인 매칭 → 도메인에 따라
└── Subject 패턴 → 패턴에 따라
```

---

## 13. 구현 계획

### Phase 1: 핵심 헤더 분류기 (Week 1-2)

**목표**: 가장 신뢰도 높은 헤더 기반 분류

1. **GitHub 헤더 파서**
   - `X-GitHub-Reason` 파싱
   - `X-GitHub-Severity` 파싱
   - CC 주소 패턴 폴백

2. **RFC 표준 헤더 강화**
   - `List-Unsubscribe-Post` 처리
   - `Auto-Submitted` 상세 분류
   - `X-Auto-Response-Suppress` 추가

3. **ESP 헤더 확장**
   - Postmark (트랜잭션 특화)
   - Customer.io, Braze 등 추가

### Phase 2: 도메인 분류기 (Week 2-3)

**목표**: 알려진 도메인 기반 빠른 분류

1. **도메인 데이터베이스**
   ```go
   type KnownDomain struct {
       Domain      string
       Category    EmailCategory
       SubCategory EmailSubCategory
       Priority    Priority
       Confidence  float64
   }
   ```

2. **초기 도메인 목록**
   - 개발자 서비스: ~50개
   - 금융/결제: ~30개
   - 쇼핑: ~40개
   - 한국 서비스: ~50개

### Phase 3: Subject 패턴 분류기 (Week 3-4)

**목표**: Subject 라인 기반 세분화

1. **패턴 매칭 엔진**
   - 정규식 기반
   - 다국어 지원 (영어/한국어)

2. **우선순위 패턴**
   - 장애/보안 패턴 (Urgent)
   - CI/CD 패턴 (성공: Low, 실패: High)
   - 주문/배송 패턴

### Phase 4: 학습 및 피드백 (Week 4+)

**목표**: 사용자 피드백 기반 개선

1. **사용자 도메인 학습**
   - 분류 수정 시 도메인 저장
   - 신뢰도 자동 조정

2. **새로운 패턴 발견**
   - LLM 분류 결과 분석
   - 새 도메인/패턴 자동 제안

---

## 참고 자료

### RFC 문서
- [RFC 5322 - Internet Message Format](https://datatracker.ietf.org/doc/html/rfc5322)
- [RFC 2369 - List-* Headers](https://www.ietf.org/rfc/rfc2369.txt)
- [RFC 8058 - List-Unsubscribe-Post](https://www.rfc-editor.org/rfc/rfc8058.html)
- [RFC 3834 - Auto-Submitted](https://datatracker.ietf.org/doc/html/rfc3834)

### 서비스 문서
- [GitHub Email Headers](https://docs.github.com/en/subscriptions-and-notifications/reference/email-notification-headers)
- [AWS SES Headers](https://docs.aws.amazon.com/ses/latest/dg/header-fields.html)
- [Google Email Guidelines](https://support.google.com/a/answer/81126)
- [SendGrid X-SMTPAPI](https://www.twilio.com/docs/sendgrid/for-developers/sending-email/building-an-x-smtpapi-header)
- [Linear Webhooks](https://linear.app/developers/webhooks)
- [Vercel Notifications](https://vercel.com/docs/notifications)
- [Sentry Notifications](https://docs.sentry.io/product/alerts/notifications/)

### 모범 사례
- [Superhuman Auto Label](https://techcrunch.com/2025/02/19/superhuman-introduces-ai-powered-categorization-to-reduce-spammy-emails-in-your-inbox/)
- [Inbox Zero Method](https://blog.superhuman.com/inbox-zero-method/)
