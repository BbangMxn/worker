// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// AWS Parser
// =============================================================================
//
// AWS email patterns:
//   - From: no-reply@sns.amazonaws.com (SNS notifications)
//   - From: *@amazon.com, *@aws.amazon.com
//   - From: noreply@marketplace.aws (Marketplace)
//   - From: aws-billing-alerts@amazon.com (Billing)
//
// Subject patterns vary by service:
//   - CodePipeline: "SUCCEEDED: AWS CodePipeline {pipeline-name}"
//   - CodePipeline: "FAILED: AWS CodePipeline {pipeline-name}"
//   - CodeBuild: "Build {status} for {project-name}"
//   - CloudWatch: "ALARM: {alarm-name} in {region}"
//   - CloudWatch: "OK: {alarm-name} in {region}"
//   - Billing: "AWS Budgets: {budget-name} has exceeded {threshold}"
//   - EC2: "EC2 Instance State-change Notification"
//   - RDS: "RDS Notification Message"
//
// SNS message format in body (JSON):
//   - Type, MessageId, TopicArn, Subject, Message, Timestamp

// AWSParser parses AWS notification emails.
type AWSParser struct {
	*DeploymentBaseParser
}

// NewAWSParser creates a new AWS parser.
func NewAWSParser() *AWSParser {
	return &AWSParser{
		DeploymentBaseParser: NewDeploymentBaseParser(ServiceAWS),
	}
}

// AWS-specific regex patterns
var (
	// Subject patterns
	awsCodePipelinePattern     = regexp.MustCompile(`(?i)(SUCCEEDED|FAILED|STARTED):\s*AWS\s*CodePipeline\s+(.+)`)
	awsCodeBuildPattern        = regexp.MustCompile(`(?i)Build\s+(Succeeded|Failed|In Progress)\s+for\s+(.+)`)
	awsCloudWatchPattern       = regexp.MustCompile(`(?i)(ALARM|OK|INSUFFICIENT_DATA):\s*["']?([^"']+)["']?\s+in\s+(.+)`)
	awsBudgetPattern           = regexp.MustCompile(`(?i)AWS\s*Budgets?:\s*(.+?)\s+has\s+exceeded`)
	awsEC2Pattern              = regexp.MustCompile(`(?i)EC2\s+Instance\s+State-?change`)
	awsRDSPattern              = regexp.MustCompile(`(?i)RDS\s+Notification`)
	awsLambdaPattern           = regexp.MustCompile(`(?i)Lambda\s+(?:function|error)`)
	awsElasticBeanstalkPattern = regexp.MustCompile(`(?i)Elastic\s*Beanstalk`)

	// URL patterns
	awsConsoleURLPattern = regexp.MustCompile(`https://(?:[a-z0-9-]+\.)?console\.aws\.amazon\.com/[^\s"<>]+`)

	// Region pattern
	awsRegionPattern = regexp.MustCompile(`(?i)(?:in\s+)?([a-z]{2}-[a-z]+-\d)`)

	// ARN pattern
	awsARNPattern = regexp.MustCompile(`arn:aws:[a-z0-9-]+:[a-z0-9-]*:\d*:[^\s"<>]+`)
)

// CanParse checks if this parser can handle the email.
func (p *AWSParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	return strings.Contains(fromLower, "@amazonaws.com") ||
		strings.Contains(fromLower, "@amazon.com") ||
		strings.Contains(fromLower, "@aws.amazon.com") ||
		strings.Contains(fromLower, "aws-") ||
		strings.Contains(fromLower, "@marketplace.aws")
}

// Parse extracts structured data from AWS emails.
func (p *AWSParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	// Detect event
	event, awsService := p.detectAWSEvent(subject)

	// Extract data
	data := p.extractData(input, awsService)

	// Calculate priority
	eventScore := p.GetEventScoreForEvent(event)
	isUrgent := event == DeployEventFailed || event == DeployEventBuildFailed || event == DeployEventBillingAlert

	priority, score := p.CalculateDeploymentPriority(DeploymentPriorityConfig{
		DomainScore: 0.35, // AWS is critical infrastructure
		EventScore:  eventScore,
		IsUrgent:    isUrgent,
	})

	// Determine category
	category, subCat := p.DetermineDeploymentCategory(event)

	// Generate action items
	actionItems := p.GenerateDeploymentActionItems(event, data)

	// Generate entities
	entities := p.GenerateDeploymentEntities(data)

	return &ParsedEmail{
		Category:      CategoryDeployment,
		Service:       ServiceAWS,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:aws:" + awsService + ":" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"aws", "service:" + awsService, "event:" + string(event)},
	}, nil
}

// detectAWSEvent detects the AWS event and service from subject.
func (p *AWSParser) detectAWSEvent(subject string) (DeploymentEventType, string) {
	// CodePipeline
	if matches := awsCodePipelinePattern.FindStringSubmatch(subject); len(matches) >= 3 {
		status := strings.ToUpper(matches[1])
		switch status {
		case "SUCCEEDED":
			return DeployEventSucceeded, "codepipeline"
		case "FAILED":
			return DeployEventFailed, "codepipeline"
		case "STARTED":
			return DeployEventStarted, "codepipeline"
		}
	}

	// CodeBuild
	if matches := awsCodeBuildPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		status := strings.ToLower(matches[1])
		switch status {
		case "succeeded":
			return DeployEventBuildPassed, "codebuild"
		case "failed":
			return DeployEventBuildFailed, "codebuild"
		case "in progress":
			return DeployEventStarted, "codebuild"
		}
	}

	// CloudWatch Alarms
	if matches := awsCloudWatchPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		status := strings.ToUpper(matches[1])
		switch status {
		case "ALARM":
			return DeployEventFailed, "cloudwatch"
		case "OK":
			return DeployEventSucceeded, "cloudwatch"
		case "INSUFFICIENT_DATA":
			return DeployEventUsageAlert, "cloudwatch"
		}
	}

	// Budget alerts
	if awsBudgetPattern.MatchString(subject) {
		return DeployEventBillingAlert, "budgets"
	}

	// EC2
	if awsEC2Pattern.MatchString(subject) {
		return DeployEventSucceeded, "ec2" // State change notification
	}

	// RDS
	if awsRDSPattern.MatchString(subject) {
		return DeployEventSucceeded, "rds"
	}

	// Lambda
	if awsLambdaPattern.MatchString(subject) {
		if strings.Contains(strings.ToLower(subject), "error") {
			return DeployEventFailed, "lambda"
		}
		return DeployEventSucceeded, "lambda"
	}

	// Elastic Beanstalk
	if awsElasticBeanstalkPattern.MatchString(subject) {
		if strings.Contains(strings.ToLower(subject), "fail") {
			return DeployEventFailed, "elasticbeanstalk"
		}
		return DeployEventSucceeded, "elasticbeanstalk"
	}

	// Fallback
	return p.DetectDeploymentEvent(subject, ""), "general"
}

// extractData extracts structured data from the email.
func (p *AWSParser) extractData(input *ParserInput, awsService string) *ExtractedData {
	data := &ExtractedData{
		Extra: make(map[string]interface{}),
	}

	if input.Message == nil {
		return data
	}

	subject := input.Message.Subject
	bodyText := ""
	if input.Body != nil {
		bodyText = input.Body.Text
		if bodyText == "" {
			bodyText = input.Body.HTML
		}
	}
	combined := subject + "\n" + bodyText

	// Store AWS service
	data.Extra["aws_service"] = awsService

	// Extract based on service type
	switch awsService {
	case "codepipeline":
		p.extractCodePipelineData(subject, combined, data)
	case "codebuild":
		p.extractCodeBuildData(subject, combined, data)
	case "cloudwatch":
		p.extractCloudWatchData(subject, combined, data)
	case "budgets":
		p.extractBudgetData(subject, combined, data)
	default:
		data.Project = p.ExtractProjectName(subject)
	}

	// Extract URL
	if matches := awsConsoleURLPattern.FindString(combined); matches != "" {
		data.URL = matches
	}

	// Extract region
	if matches := awsRegionPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Extra["region"] = matches[1]
	}

	// Extract ARN
	if matches := awsARNPattern.FindString(combined); matches != "" {
		data.Extra["arn"] = matches
	}

	// Set title
	if data.Title == "" {
		data.Title = subject
	}

	return data
}

// extractCodePipelineData extracts CodePipeline-specific data.
func (p *AWSParser) extractCodePipelineData(subject, combined string, data *ExtractedData) {
	if matches := awsCodePipelinePattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Project = strings.TrimSpace(matches[2])
		data.PipelineID = data.Project
		data.Title = "CodePipeline: " + data.Project
	}
}

// extractCodeBuildData extracts CodeBuild-specific data.
func (p *AWSParser) extractCodeBuildData(subject, combined string, data *ExtractedData) {
	if matches := awsCodeBuildPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Project = strings.TrimSpace(matches[2])
		data.BuildStatus = strings.ToLower(matches[1])
		data.Title = "CodeBuild: " + data.Project
	}
}

// extractCloudWatchData extracts CloudWatch-specific data.
func (p *AWSParser) extractCloudWatchData(subject, combined string, data *ExtractedData) {
	if matches := awsCloudWatchPattern.FindStringSubmatch(subject); len(matches) >= 4 {
		data.AlertStatus = strings.ToLower(matches[1])
		data.Title = strings.TrimSpace(matches[2])
		data.Extra["region"] = strings.TrimSpace(matches[3])
	}
}

// extractBudgetData extracts Budget-specific data.
func (p *AWSParser) extractBudgetData(subject, combined string, data *ExtractedData) {
	if matches := awsBudgetPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Title = strings.TrimSpace(matches[1])
	}

	// Look for amount in body
	amountPattern := regexp.MustCompile(`\$([0-9,.]+)`)
	if matches := amountPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Amount = "$" + matches[1]
	}
}
