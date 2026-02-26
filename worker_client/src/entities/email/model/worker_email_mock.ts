import type { Email, EmailLabel } from "./worker_email_types";

export const mockLabels: EmailLabel[] = [
  { id: "l1", name: "Work", color: "#1a73e8" },
  { id: "l2", name: "Design", color: "#34a853" },
  { id: "l3", name: "GitHub", color: "#6e7781" },
  { id: "l4", name: "Newsletter", color: "#9333ea" },
];

export const mockEmails: Email[] = [
  {
    id: 1,
    connectionId: 1,
    externalId: "ext_001",
    fromEmail: "sarah@company.com",
    fromName: "Sarah Chen",
    toEmails: ["me@company.com"],
    subject: "Q4 Product Roadmap Review",
    snippet:
      "Hi team, I wanted to share the updated roadmap for Q4. Please review and let me know your thoughts...",
    body: `Hi team,

I wanted to share the updated roadmap for Q4. Please review and let me know your thoughts by end of week.

Key highlights:
- Mobile app redesign
- API v2 launch
- Performance improvements

Best,
Sarah`,
    folder: "inbox",
    tags: ["starred", "important"],
    labels: ["Work"],
    isRead: false,
    hasAttachments: true,
    attachments: [
      {
        id: "att_1",
        name: "roadmap.pdf",
        mimeType: "application/pdf",
        size: 2400000,
        url: "#",
      },
    ],
    workflowStatus: "todo",
    aiStatus: "completed",
    aiCategory: "work",
    aiPriority: "high",
    aiSummary: "Q4 roadmap review request with key product highlights",
    receivedAt: new Date(Date.now() - 1000 * 60 * 15).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 15).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 15).toISOString(),
  },
  {
    id: 2,
    connectionId: 1,
    externalId: "ext_002",
    fromEmail: "alex@design.co",
    fromName: "Alex Kim",
    toEmails: ["me@company.com"],
    subject: "Design system updates",
    snippet:
      "Hey! Just pushed the latest component updates to Figma. The new button variants are ready...",
    body: `Hey!

Just pushed the latest component updates to Figma. The new button variants are ready for review.

Changes include:
- Updated color tokens
- New icon set
- Improved spacing

Let me know if you have any questions!

Alex`,
    folder: "inbox",
    tags: [],
    labels: ["Design"],
    isRead: false,
    hasAttachments: false,
    workflowStatus: "inbox",
    aiStatus: "completed",
    aiCategory: "work",
    aiPriority: "normal",
    aiSummary: "Design system component updates in Figma",
    receivedAt: new Date(Date.now() - 1000 * 60 * 60 * 2).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 60 * 2).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 60 * 2).toISOString(),
  },
  {
    id: 3,
    connectionId: 1,
    externalId: "ext_003",
    fromEmail: "mike@startup.io",
    fromName: "Mike Johnson",
    toEmails: ["me@company.com"],
    subject: "Partnership opportunity",
    snippet:
      "Hi, I came across your product and would love to explore a potential partnership...",
    body: `Hi,

I came across your product and would love to explore a potential partnership opportunity.

We're building something complementary and I think there could be great synergy here.

Would you be open to a quick call next week?

Best regards,
Mike`,
    folder: "inbox",
    tags: [],
    labels: [],
    isRead: true,
    hasAttachments: false,
    workflowStatus: "done",
    aiStatus: "completed",
    aiCategory: "work",
    aiPriority: "normal",
    receivedAt: new Date(Date.now() - 1000 * 60 * 60 * 5).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 60 * 5).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 60 * 5).toISOString(),
  },
  {
    id: 4,
    connectionId: 1,
    externalId: "ext_004",
    fromEmail: "noreply@github.com",
    fromName: "GitHub",
    toEmails: ["me@company.com"],
    subject: "[worker] Pull request merged: feat/email-view",
    snippet: "Your pull request #142 has been merged into main by @teammate...",
    body: `Your pull request #142 has been merged into main by @teammate.

Commit: feat/email-view - Add new email detail component

View on GitHub: https://github.com/...`,
    folder: "inbox",
    tags: [],
    labels: ["GitHub"],
    isRead: true,
    hasAttachments: false,
    workflowStatus: "done",
    aiStatus: "completed",
    aiCategory: "news",
    aiPriority: "low",
    receivedAt: new Date(Date.now() - 1000 * 60 * 60 * 8).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 60 * 8).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 60 * 8).toISOString(),
  },
  {
    id: 5,
    connectionId: 1,
    externalId: "ext_005",
    fromEmail: "emma@team.com",
    fromName: "Emma Wilson",
    toEmails: ["me@company.com"],
    subject: "Team lunch tomorrow?",
    snippet:
      "Hey! A few of us are planning to grab lunch tomorrow at that new place downtown...",
    body: `Hey!

A few of us are planning to grab lunch tomorrow at that new place downtown. Want to join?

We're thinking around 12:30.

Let me know!
Emma`,
    folder: "inbox",
    tags: ["starred"],
    labels: [],
    isRead: true,
    hasAttachments: false,
    workflowStatus: "inbox",
    aiStatus: "completed",
    aiCategory: "personal",
    aiPriority: "normal",
    receivedAt: new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString(),
  },
  {
    id: 6,
    connectionId: 1,
    externalId: "ext_006",
    fromEmail: "team@notion.so",
    fromName: "Notion",
    toEmails: ["me@company.com"],
    subject: "Your weekly summary",
    snippet:
      "Here is your activity summary for the past week. You completed 12 tasks...",
    body: `Here's your activity summary for the past week:

- Completed tasks: 12
- Pages created: 5
- Comments added: 8

Keep up the great work!

The Notion Team`,
    folder: "inbox",
    tags: [],
    labels: ["Newsletter"],
    isRead: true,
    hasAttachments: false,
    workflowStatus: "done",
    aiStatus: "completed",
    aiCategory: "promo",
    aiPriority: "low",
    receivedAt: new Date(Date.now() - 1000 * 60 * 60 * 36).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 60 * 36).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 60 * 36).toISOString(),
  },
  {
    id: 7,
    connectionId: 1,
    externalId: "ext_007",
    fromEmail: "cto@company.com",
    fromName: "David Park",
    toEmails: ["me@company.com"],
    subject: "Urgent: Production issue",
    snippet:
      "We have a critical issue in production that needs immediate attention...",
    body: `Team,

We have a critical issue in production that needs immediate attention. The payment service is experiencing intermittent failures.

Please join the incident call ASAP.

David`,
    folder: "inbox",
    tags: ["important"],
    labels: ["Work"],
    isRead: false,
    hasAttachments: false,
    workflowStatus: "todo",
    aiStatus: "completed",
    aiCategory: "work",
    aiPriority: "urgent",
    aiSummary:
      "Critical production issue with payment service requiring immediate attention",
    receivedAt: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
  },
  {
    id: 8,
    connectionId: 1,
    externalId: "ext_008",
    fromEmail: "hr@company.com",
    fromName: "HR Team",
    toEmails: ["me@company.com"],
    subject: "연차 신청 승인 완료",
    snippet:
      "신청하신 연차 휴가가 승인되었습니다. 1월 15일 휴가 사용이 확정되었습니다...",
    body: `안녕하세요,

신청하신 연차 휴가가 승인되었습니다.

- 휴가일: 2024년 1월 15일 (월)
- 유형: 연차

즐거운 휴가 보내세요!

인사팀 드림`,
    folder: "inbox",
    tags: [],
    labels: [],
    isRead: true,
    hasAttachments: false,
    workflowStatus: "done",
    aiStatus: "completed",
    aiCategory: "work",
    aiPriority: "normal",
    receivedAt: new Date(Date.now() - 1000 * 60 * 60 * 48).toISOString(),
    createdAt: new Date(Date.now() - 1000 * 60 * 60 * 48).toISOString(),
    updatedAt: new Date(Date.now() - 1000 * 60 * 60 * 48).toISOString(),
  },
];
