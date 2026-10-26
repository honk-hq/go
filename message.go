package honk

import (
	"strings"
	"time"
)

// Severity of a message, lowest to highest: the level alone decides how loudly a push
// interrupts. Every severity has a horn name on the Honk scale (Light, Beep, Loud, Long,
// Blast); those constants are aliases of the canonical values.
type Severity string

// The Honk scale: horn names, aliases of the canonical severities (honk.Loud == honk.SeverityWarning).
const (
	Light Severity = SeverityInfo     // Light
	Beep  Severity = SeveritySuccess  // Beep
	Loud  Severity = SeverityWarning  // Loud
	Long  Severity = SeverityError    // Long
	Blast Severity = SeverityCritical // Blast
)

// Canonical severities, as stored and returned by the server.
const (
	SeverityInfo     Severity = "info"
	SeveritySuccess  Severity = "success"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// SeverityAliases maps the horn names to the canonical severities.
var SeverityAliases = map[string]Severity{
	"light": SeverityInfo, "beep": SeveritySuccess, "loud": SeverityWarning, "long": SeverityError, "blast": SeverityCritical,
}

// ParseSeverity returns the canonical severity for a canonical value or horn alias,
// case-insensitively: ParseSeverity("LOUD") == SeverityWarning.
func ParseSeverity(s string) (Severity, bool) {
	v := strings.ToLower(strings.TrimSpace(s))
	for _, c := range severities {
		if string(c) == v {
			return c, true
		}
	}
	c, ok := SeverityAliases[v]
	return c, ok
}

// Priority declared by the source. Urgent is kept only from an ingestion key with
// allow_urgent; from any other key the server lowers it to high (a "downgraded" warning).
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// EventType: a problem opens an incident for its group, a recovery closes it. Without a
// GroupKey a message groups by its title.
type EventType string

const (
	EventTypeEvent    EventType = "event"
	EventTypeProblem  EventType = "problem"
	EventTypeRecovery EventType = "recovery"
)

// Category from taxonomy v1.
type Category string

const (
	CategoryInfrastructure Category = "infrastructure"
	CategorySecurity       Category = "security"
	CategoryBackups        Category = "backups"
	CategoryDeployments    Category = "deployments"
	CategoryPayments       Category = "payments"
	CategoryCustomers      Category = "customers"
	CategorySales          Category = "sales"
	CategoryAutomation     Category = "automation"
	CategoryPersonal       Category = "personal"
	CategoryOther          Category = "other"
)

var severities = []Severity{SeverityInfo, SeveritySuccess, SeverityWarning, SeverityError, SeverityCritical}

// Message is one event for POST /v1/messages. A Title or a Message is required; zero values
// are omitted, so the server defaults apply (severity info, priority normal, source "api",
// environment "default", channel "general", event type event, TTL 3600 s).
//
// The SDK sends the fields as given. The server shortens, normalizes or leaves out what it
// cannot use and accepts the message anyway; Accepted.Warnings says what it changed.
type Message struct {
	// Title is one line (≤ 160 characters). Defaults to the first line of Message.
	Title string
	// Message is plain text (≤ 8192 bytes). Line breaks and tabs are kept.
	Message string
	// Severity is the level: a horn name (Light, Beep, Loud, Long, Blast) or canonical value,
	// any case. Sent canonical. Default light (info).
	Severity Severity
	Priority Priority
	Category Category
	// Source, Environment and Channel default to the client's Defaults.
	Source      string
	Environment string
	Channel     string
	// GroupKey relates occurrences (optional): messages with the same key (per environment,
	// source and channel) form one group, so the first one pushes and repeats update it calmly.
	// Without one a message groups by its title.
	GroupKey  string
	EventType EventType
	// OccurredAt is when it happened at the source (informational). Zero means unset.
	OccurredAt time.Time
	// URL is an https link shown as "Open link".
	URL string
	// ImageURL is an https image the server fetches after ingestion.
	ImageURL string
	// Actions are up to 3 buttons, in display order (the first is the primary).
	Actions []Action
	// Metadata: up to 16 keys; values are strings, numbers or booleans.
	Metadata map[string]any
	// TTLSeconds is the push lifetime, 60–86400. Zero means the server default (3600).
	TTLSeconds int
	// SourceSequence is a monotonic counter per source stream so a delayed recovery can never
	// close a newer problem. Use honk.Ptr(n).
	SourceSequence *int64
}

// Action is a button on a message. Honk never opens or fetches the URL; the app opens it when
// the user taps the button.
type Action struct {
	// Title is shown as sent (≤ 40 characters): "Reply", "Call Maria".
	Title string `json:"title"`
	// URL is https://, mailto:, tel: or sms:. The server leaves out an action it cannot use
	// (an "ignored" warning) and keeps the others.
	URL string `json:"url"`
}

// Defaults are applied to every message that leaves these fields empty.
type Defaults struct {
	Source      string
	Environment string
	Channel     string
}

// Accepted is the 202 answer: the message is durably stored (not necessarily pushed yet).
type Accepted struct {
	// ID of the message (msg_…). The original ID when Duplicate is true.
	ID string `json:"id"`
	// Duplicate is true when this idempotency key was already accepted with the same payload
	// in the last 24 hours.
	Duplicate bool `json:"duplicate"`
	// ReceivedAt is when the server accepted it (the first time, for a duplicate).
	ReceivedAt time.Time `json:"received_at"`
	// Warnings lists what the server corrected before accepting the message (a title
	// shortened, an invalid URL left out, urgent lowered to high, …). Empty when nothing.
	Warnings []Warning `json:"warnings"`
}

// Warning is one correction the server made to an accepted message. Warnings are not errors:
// the message was delivered. Log them so you can fix the sender.
type Warning struct {
	// Field is the wire name: "title", "metadata.order", "actions[1].url", …
	Field string `json:"field"`
	// Code is one of the Warning* constants (other values may be added).
	Code string `json:"code"`
	// Message says, in English, what was wrong and what was done.
	Message string `json:"message"`
}

func (w Warning) String() string {
	s := w.Field + " " + w.Code
	if w.Message != "" {
		s += ": " + w.Message
	}
	return s
}

// Warning codes (Warning.Code).
const (
	WarningTruncated  = "truncated"  // shortened to the limit ("…" for text)
	WarningNormalized = "normalized" // rewritten into an accepted form (one line, control characters, type)
	WarningIgnored    = "ignored"    // an invalid value left out; the default applies
	WarningClamped    = "clamped"    // a number moved into its range
	WarningDowngraded = "downgraded" // a priority the key may not send, lowered
	WarningHashed     = "hashed"     // a long group key shortened with a hash
)

// Ptr returns a pointer to v, e.g. Message{SourceSequence: honk.Ptr[int64](42)}.
func Ptr[T any](v T) *T { return &v }

// wireMessage is the exact JSON of contracts/openapi.yaml MessageRequest.
type wireMessage struct {
	Title          string         `json:"title,omitempty"`
	Message        string         `json:"message,omitempty"`
	Severity       Severity       `json:"severity,omitempty"`
	Priority       Priority       `json:"priority,omitempty"`
	Category       Category       `json:"category,omitempty"`
	Source         string         `json:"source,omitempty"`
	Environment    string         `json:"environment,omitempty"`
	Channel        string         `json:"channel,omitempty"`
	GroupKey       string         `json:"group_key,omitempty"`
	EventType      EventType      `json:"event_type,omitempty"`
	OccurredAt     string         `json:"occurred_at,omitempty"`
	URL            string         `json:"url,omitempty"`
	ImageURL       string         `json:"image_url,omitempty"`
	Actions        []Action       `json:"actions,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	TTLSeconds     int            `json:"ttl_seconds,omitempty"`
	SourceSequence *int64         `json:"source_sequence,omitempty"`
}

// normalized applies the defaults and turns a horn alias into the canonical severity, so the
// idempotency payload is canonical and older servers understand it.
func (m Message) normalized(d Defaults) Message {
	if s, ok := ParseSeverity(string(m.Severity)); ok {
		m.Severity = s
	}
	return m.withDefaults(d)
}

func (m Message) withDefaults(d Defaults) Message {
	if m.Source == "" {
		m.Source = d.Source
	}
	if m.Environment == "" {
		m.Environment = d.Environment
	}
	if m.Channel == "" {
		m.Channel = d.Channel
	}
	return m
}

func (m Message) wire() wireMessage {
	w := wireMessage{
		Title: m.Title, Message: m.Message, Severity: m.Severity, Priority: m.Priority,
		Category: m.Category, Source: m.Source, Environment: m.Environment, Channel: m.Channel,
		GroupKey: m.GroupKey, EventType: m.EventType, URL: m.URL, ImageURL: m.ImageURL,
		Actions: m.Actions, TTLSeconds: m.TTLSeconds, SourceSequence: m.SourceSequence,
	}
	if !m.OccurredAt.IsZero() {
		w.OccurredAt = m.OccurredAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	}
	if len(m.Metadata) > 0 {
		w.Metadata = m.Metadata
	}
	return w
}
