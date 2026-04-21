package classifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/invopop/jsonschema"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

const requestTimeout = 15 * time.Second

type ContactSubmission struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Comment string `json:"comment"`
}

type Classification struct {
	RealInterestScore int    `json:"real_interest_score"`
	SpamScore         int    `json:"spam_score"`
	SalesPitchScore   int    `json:"sales_pitch_score"`
	FinalLabel        string `json:"final_label"`
	Confidence        int    `json:"confidence"`
	ReasonShort       string `json:"reason_short"`
	UsedModel         bool   `json:"used_model"`
}

type ModelOutput struct {
	RealInterestScore int    `json:"real_interest_score" jsonschema:"minimum=1,maximum=10"`
	SpamScore         int    `json:"spam_score" jsonschema:"minimum=1,maximum=10"`
	SalesPitchScore   int    `json:"sales_pitch_score" jsonschema:"minimum=1,maximum=10"`
	FinalLabel        string `json:"final_label" jsonschema:"enum=real_interest,enum=spam,enum=sales_pitch"`
	Confidence        int    `json:"confidence" jsonschema:"minimum=1,maximum=10"`
	ReasonShort       string `json:"reason_short"`
}

type Classifier struct {
	client  openai.Client
	model   string
	Verbose bool
}

func New(apiKey, model string) *Classifier {
	if model == "" {
		model = "gpt-5.4-nano"
	}
	return &Classifier{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (c *Classifier) Classify(ctx context.Context, submission ContactSubmission) (Classification, error) {
	if err := validateSubmission(submission); err != nil {
		return Classification{}, err
	}

	if pf := applyPrefilter(submission); pf.Matched {
		if c.Verbose {
			log.Printf("[classifier] prefilter matched: label=%s reason=%s", pf.Result.FinalLabel, pf.Result.ReasonShort)
		}
		return pf.Result, nil
	}

	if c.Verbose {
		log.Printf("[classifier] no prefilter match, calling OpenAI model=%s", c.model)
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	return c.classifyWithOpenAI(ctx, submission)
}

type prefilterResult struct {
	Matched bool
	Result  Classification
}

func validateSubmission(s ContactSubmission) error {
	if strings.TrimSpace(s.Comment) == "" {
		return errors.New("comment is required")
	}
	if utf8.RuneCountInString(strings.TrimSpace(s.Comment)) > 10000 {
		return errors.New("comment too long")
	}
	if strings.TrimSpace(s.Email) == "" {
		return errors.New("email is required")
	}
	if _, err := mail.ParseAddress(s.Email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}
	return nil
}

func applyPrefilter(s ContactSubmission) prefilterResult {
	text := normalizeForRules(s)

	salesPhrases := []string{
		"improve your seo",
		"boost your seo",
		"generate more leads",
		"lead generation",
		"book a quick call",
		"open to a quick call",
		"we help companies like yours",
		"digital marketing agency",
		"web design services",
		"link building",
		"demand generation",
		"cold email outreach",
		"software development services",
		"staff augmentation",
		"hire our team",
		"can i send you more information",
	}

	spamPhrases := []string{
		"viagra",
		"casino",
		"crypto giveaway",
		"free bitcoin",
		"loan approved",
		"earn money fast",
		"click here",
		"telegram",
		"whatsapp us",
		"adult dating",
		"investment opportunity",
	}

	urlCount := countURLs(text)
	nonWordRatio := calcNonWordRatio(text)

	if urlCount >= 4 || nonWordRatio > 0.45 || looksLikeGarbage(text) {
		return prefilterResult{
			Matched: true,
			Result: Classification{
				RealInterestScore: 1,
				SpamScore:         10,
				SalesPitchScore:   2,
				FinalLabel:        "spam",
				Confidence:        9,
				ReasonShort:       "Rule-based prefilter flagged obvious junk or bulk spam patterns.",
				UsedModel:         false,
			},
		}
	}

	for _, phrase := range salesPhrases {
		if strings.Contains(text, phrase) {
			return prefilterResult{
				Matched: true,
				Result: Classification{
					RealInterestScore: 2,
					SpamScore:         2,
					SalesPitchScore:   9,
					FinalLabel:        "sales_pitch",
					Confidence:        8,
					ReasonShort:       "Rule-based prefilter matched a common outbound sales phrase.",
					UsedModel:         false,
				},
			}
		}
	}

	for _, phrase := range spamPhrases {
		if strings.Contains(text, phrase) {
			return prefilterResult{
				Matched: true,
				Result: Classification{
					RealInterestScore: 1,
					SpamScore:         10,
					SalesPitchScore:   1,
					FinalLabel:        "spam",
					Confidence:        9,
					ReasonShort:       "Rule-based prefilter matched a common spam phrase.",
					UsedModel:         false,
				},
			}
		}
	}

	return prefilterResult{}
}

func (c *Classifier) classifyWithOpenAI(ctx context.Context, submission ContactSubmission) (Classification, error) {
	userPrompt := buildUserPrompt(submission)
	if c.Verbose {
		log.Printf("[classifier] OpenAI request: model=%s", c.model)
		log.Printf("[classifier] user prompt: %s", userPrompt)
	}

	resp, err := c.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        c.model,
		Instructions: param.NewOpt(buildSystemPrompt()),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfMessage(
					userPrompt,
					responses.EasyInputMessageRoleUser,
				),
			},
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:        "contact_form_classifier",
					Description: param.NewOpt("Classification for inbound contact form submissions"),
					Schema:      generateSchema[ModelOutput](),
					Strict:      param.NewOpt(true),
				},
			},
		},
		Temperature: param.NewOpt(0.0),
	})
	if err != nil {
		if c.Verbose {
			log.Printf("[classifier] OpenAI error: %v", err)
		}
		return Classification{}, fmt.Errorf("openai request failed: %w", err)
	}

	raw := strings.TrimSpace(resp.OutputText())
	if c.Verbose {
		log.Printf("[classifier] OpenAI response: %s", raw)
	}
	if raw == "" {
		return Classification{}, errors.New("openai returned empty structured output")
	}

	var parsed ModelOutput
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return Classification{}, fmt.Errorf("failed to parse model output: %w; raw=%q", err, raw)
	}

	if err := validateModelOutput(parsed); err != nil {
		return Classification{}, err
	}

	return Classification{
		RealInterestScore: parsed.RealInterestScore,
		SpamScore:         parsed.SpamScore,
		SalesPitchScore:   parsed.SalesPitchScore,
		FinalLabel:        parsed.FinalLabel,
		Confidence:        parsed.Confidence,
		ReasonShort:       parsed.ReasonShort,
		UsedModel:         true,
	}, nil
}

func buildSystemPrompt() string {
	return strings.TrimSpace(`
You classify inbound "Contact Us" form submissions.

Return JSON only, following the provided schema exactly.

Fields provided:
- Name
- Email
- Comment

Scoring rules:
- real_interest_score: 1-10 likelihood this is legitimate inbound interest.
- spam_score: 1-10 likelihood this is junk, scam, bot, phishing, or irrelevant spam.
- sales_pitch_score: 1-10 likelihood the sender is trying to sell us something.

Definitions:
- real_interest:
  A real prospective customer, partner, support request, or relevant business inquiry.
- spam:
  Nonsense, scams, phishing, bot garbage, irrelevant promotions, or obviously mass-posted junk.
- sales_pitch:
  The sender is attempting to sell us marketing, SEO, development services, recruiting, lead gen, software, outsourcing, or similar services.

Decision rules:
- Score all three independently.
- final_label must be whichever category is strongest overall.
- Prefer sales_pitch over real_interest when the message is clearly trying to sell us something.
- Prefer spam when the message is obviously junk, malicious, or irrelevant.
- Keep reason_short to one concise sentence.
- Do not invent facts.
- Be conservative with 10s unless it is very clear.
`)
}

func buildUserPrompt(s ContactSubmission) string {
	b, _ := json.MarshalIndent(s, "", "  ")
	return "Classify this contact submission:\n\n" + string(b)
}

func validateModelOutput(v ModelOutput) error {
	if v.RealInterestScore < 1 || v.RealInterestScore > 10 {
		return fmt.Errorf("invalid real_interest_score: %d", v.RealInterestScore)
	}
	if v.SpamScore < 1 || v.SpamScore > 10 {
		return fmt.Errorf("invalid spam_score: %d", v.SpamScore)
	}
	if v.SalesPitchScore < 1 || v.SalesPitchScore > 10 {
		return fmt.Errorf("invalid sales_pitch_score: %d", v.SalesPitchScore)
	}
	if v.Confidence < 1 || v.Confidence > 10 {
		return fmt.Errorf("invalid confidence: %d", v.Confidence)
	}
	switch v.FinalLabel {
	case "real_interest", "spam", "sales_pitch":
	default:
		return fmt.Errorf("invalid final_label: %q", v.FinalLabel)
	}
	if strings.TrimSpace(v.ReasonShort) == "" {
		return errors.New("reason_short is required")
	}
	return nil
}

func normalizeForRules(s ContactSubmission) string {
	return strings.ToLower(strings.Join([]string{s.Name, s.Email, s.Comment}, " "))
}

var urlRE = regexp.MustCompile(`https?://|www\.`)

func countURLs(s string) int {
	return len(urlRE.FindAllStringIndex(s, -1))
}

func looksLikeGarbage(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.Count(s, "!!!") > 1 || strings.Count(s, "$$$") > 0 {
		return true
	}
	words := strings.Fields(s)
	if len(words) >= 6 {
		repeated := 0
		for i := 1; i < len(words); i++ {
			if words[i] == words[i-1] {
				repeated++
			}
		}
		if repeated >= len(words)/3 {
			return true
		}
	}
	return false
}

func calcNonWordRatio(s string) float64 {
	if s == "" {
		return 0
	}
	var total, weird int
	for _, r := range s {
		total++
		if !(r == ' ' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == ',' || r == '?' || r == '!' ||
			r == ':' || r == ';' || r == '-' || r == '_' ||
			r == '@' || r == '/' || r == '(' || r == ')' ||
			r == '\'' || r == '"' || r == '&') {
			weird++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(weird) / float64(total)
}

func generateSchema[T any]() map[string]any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	b, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}
