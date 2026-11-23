package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const npuAlphabetRetryInstruction = "Explain the decisive condition in one short Russian sentence of 8 to 16 words. End with a period. Paraphrase without quoting either source. Use only Russian letters and ASCII Latin letters for technical terms."

var npuProfilePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var npuRetryExplanation = regexp.MustCompile(`^[А-Яа-яЁёA-Za-z0-9 .,!?():;%«»/—–-]{1,319}[.!?]$`)

func npuAlphabetRetrySchema() map[string]any {
	schema := findingSchema()
	properties := schema["properties"].(map[string]any)
	properties["leftId"] = map[string]any{"const": "left"}
	properties["rightId"] = map[string]any{"const": "right"}
	properties["leftVersion"] = map[string]any{"const": 1}
	properties["rightVersion"] = map[string]any{"const": 1}
	properties["explanation"] = map[string]any{"type": "string", "pattern": npuRetryExplanation.String()}
	return schema
}

func npuPairProvenance(spec generationProvenance) (string, string) {
	spec.SystemPrompt = systemPrompt
	spec.ResponseSchema = findingSchema()
	spec.AlphabetRetrySchema = npuAlphabetRetrySchema()
	spec.Normalization = "source-aliases-v2:text-order"
	return encodePairProvenance(spec, npuAlphabetRetryInstruction)
}

func compareNPUPair(ctx context.Context, pair Pair, generate func(context.Context, Pair, string, map[string]any) (Finding, error)) (Finding, error) {
	// Opaque IDs and revision numbers are references, not requirement semantics.
	// Keep their per-request mapping in the adapter so incidental identifiers do
	// not influence the model's verdict. Source texts remain byte-for-byte intact.
	aliases := Pair{
		Left:              Source{ID: "left", Version: 1, Text: pair.Left.Text},
		Right:             Source{ID: "right", Version: 1, Text: pair.Right.Text},
		Preclassification: pair.Preclassification,
	}
	// Contradiction is symmetric. Canonical text order keeps the same comparison
	// independent of the caller's source order while preserving every source byte.
	if aliases.Right.Text < aliases.Left.Text {
		aliases.Left.Text, aliases.Right.Text = aliases.Right.Text, aliases.Left.Text
	}
	finding, err := generatePairFinding(ctx, npuAlphabetRetryInstruction, func(prompt string, retry bool) (Finding, error) {
		schema := findingSchema()
		if retry {
			schema = npuAlphabetRetrySchema()
		}
		finding, err := generate(ctx, aliases, prompt, schema)
		if err == nil && retry && (!npuRetryExplanation.MatchString(finding.Explanation) || strings.IndexFunc(finding.Explanation, unicode.IsLetter) < 0) {
			return Finding{}, invalidModelOutput("NPU retry explanation is invalid")
		}
		return finding, err
	})
	if err != nil {
		return Finding{}, err
	}
	finding.LeftID, finding.LeftVersion = pair.Left.ID, pair.Left.Version
	finding.RightID, finding.RightVersion = pair.Right.ID, pair.Right.Version
	if !validFinding(pair, finding) {
		return Finding{}, errors.New("invalid NPU comparison source mapping")
	}
	if err := ctx.Err(); err != nil {
		return Finding{}, err
	}
	return finding, nil
}

// The direct runtime parses Go's request JSON and then runs json.dumps with
// ensure_ascii=False and default separators before tokenization. Preserve that
// presentation in the managed chat message, including the order of fields.
func npuUserMessage(pair Pair) (string, error) {
	source := func(s Source) string {
		return `{"id": ` + npuJSONString(s.ID) + `, "version": ` + strconv.Itoa(s.Version) + `, "text": ` + npuJSONString(s.Text) + "}"
	}
	hint := ""
	if pre := pair.Preclassification; pre != nil {
		if !validLayaScores(pre.Decision, pre.Scores) {
			return "", errors.New("invalid comparison preclassification")
		}
		fields := make([]string, 0, 3)
		// json.Marshal sorts map keys. The Python decoder preserves that order.
		for _, label := range []string{"related", "uncertain", "unrelated"} {
			v := pre.Scores[label]
			number := "0"
			if v != 0 {
				format := byte('f')
				if v < 1e-4 {
					format = 'e'
				}
				number = strconv.FormatFloat(v, format, -1, 64)
			}
			fields = append(fields, npuJSONString(label)+": "+number)
		}
		hint = `"preclassification": {"decision": ` + npuJSONString(pre.Decision) + `, "scores": {` + strings.Join(fields, ", ") + `}}, `
	}
	return "{" + hint + `"left": ` + source(pair.Left) + `, "right": ` + source(pair.Right) + "}", nil
}

// encoding/json escapes HTML and JavaScript line separators, while Python
// emits those runes literally. Consume escaped backslashes as pairs so literal
// source strings such as "\\u2028" survive unchanged.
var npuJSONEscapes = strings.NewReplacer(
	`\\`, `\\`,
	`\u003c`, "<", `\u003e`, ">", `\u0026`, "&",
	`\u2028`, "\u2028", `\u2029`, "\u2029",
)

func npuJSONString(s string) string {
	quoted, _ := json.Marshal(s)
	return npuJSONEscapes.Replace(string(quoted))
}
