package notifications

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	"time"

	_ "time/tzdata"
)

const approvalEmailHTML = `<!doctype html>
<html><body style="margin:0;background:#f5f7f2;font-family:Arial,sans-serif;color:#203b2c">
  <main style="max-width:600px;margin:24px auto;padding:28px;background:#ffffff;border:1px solid #dfe7dc">
    <p style="color:#557a38;font-weight:bold;letter-spacing:.12em">MMSU TENNIS CLUB</p>
    <h1 style="font-size:24px">Your booking is approved</h1>
    <p>Your booking request has been approved by a club admin.</p>
    <p><strong>Training program:</strong> {{.ProgramName}}<br>
       <strong>Date and time:</strong> {{.DateTime}}<br>
       <strong>Location:</strong> {{.Location}}<br>
       <strong>Request reference:</strong> {{.SupportReference}}</p>
    <p>This email supplements, and does not replace, personal confirmation from your club admin. Please follow up with the admin for other information.</p>
    <p style="color:#68766e;font-size:12px">This notice was sent to the email address supplied with the request. Contact details are not used as identity verification.</p>
  </main>
</body></html>`

type approvalTemplateData struct {
	ProgramName      string
	DateTime         string
	Location         string
	SupportReference string
}

func BuildApprovalEmail(fromEmail string, claim Claim) (Email, error) {
	location, err := time.LoadLocation("Asia/Manila")
	if err != nil {
		return Email{}, fmt.Errorf("load club time zone: %w", err)
	}
	startsAt := claim.StartsAt.In(location)
	endsAt := claim.EndsAt.In(location)
	dateTime := startsAt.Format("Monday, 2 January 2006, 3:04 PM")
	if startsAt.Format("2006-01-02") == endsAt.Format("2006-01-02") {
		dateTime += "–" + endsAt.Format("3:04 PM")
	} else {
		dateTime += "–" + endsAt.Format("Monday, 2 January 2006, 3:04 PM")
	}
	dateTime += " (Asia/Manila)"

	templateData := approvalTemplateData{
		ProgramName:      claim.ProgramName,
		DateTime:         dateTime,
		Location:         claim.Location,
		SupportReference: claim.SupportReference,
	}
	parsed, err := htmltemplate.New("approval").Parse(approvalEmailHTML)
	if err != nil {
		return Email{}, fmt.Errorf("parse approval email template: %w", err)
	}
	var body bytes.Buffer
	if err := parsed.Execute(&body, templateData); err != nil {
		return Email{}, fmt.Errorf("render approval email template: %w", err)
	}

	text := strings.Join([]string{
		"MMSU Tennis Club",
		"Your booking is approved.",
		"",
		"Training program: " + claim.ProgramName,
		"Date and time: " + dateTime,
		"Location: " + claim.Location,
		"Request reference: " + claim.SupportReference,
		"",
		"This email supplements, and does not replace, personal confirmation from your club admin. Please follow up with the admin for direct confirmation.",
		"This notice was sent to the email address supplied with the request. Contact details are not used as identity verification.",
	}, "\n")

	return Email{
		From:    "MMSU Tennis Club <" + fromEmail + ">",
		To:      claim.RecipientEmail,
		Subject: "MMSU Tennis Club booking approved",
		HTML:    body.String(),
		Text:    text,
	}, nil
}
