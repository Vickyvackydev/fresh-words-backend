package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fresh-words-backend/config"
	"fresh-words-backend/db"
	"fresh-words-backend/models"
)

var languageNames = map[string]string{
	"en": "English",
	"fr": "French",
	"es": "Spanish",
	"yo": "Yoruba",
	"ig": "Igbo",
	"ha": "Hausa",
	"pt": "Portuguese",
	"de": "German",
	"sw": "Swahili",
	"am": "Amharic",
}

func GetLanguageName(code string) string {
	if name, ok := languageNames[strings.ToLower(code)]; ok {
		return name
	}
	return code
}

// Data structures for JSON communication with Gemini
type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature      float64 `json:"temperature"`
	ResponseMimeType string  `json:"responseMimeType"`
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type DevotionalTranslationPayload struct {
	Title              string `json:"title"`
	ScriptureQuote     string `json:"scripture_quote"`
	ScriptureReference string `json:"scripture_reference"`
	Body               string `json:"body"`
	Prayer             string `json:"prayer"`
	Reflection         string `json:"reflection"`
}

type HymnTranslationPayload struct {
	Title  string   `json:"title"`
	Chorus string   `json:"chorus"`
	Verses []string `json:"verses"`
}

// CallGeminiJSON sends a prompt to Gemini requesting a JSON response.
// If the primary model fails or is unavailable (e.g. 404 or 503), it automatically
// falls back through alternative active Gemini models.
func CallGeminiJSON(prompt string) (string, error) {
	apiKey := strings.TrimSpace(config.AppConfig.GeminiAPIKey)
	if apiKey == "" {
		return "", errors.New("GEMINI_API_KEY is not configured in backend")
	}

	primaryModel := strings.TrimSpace(config.AppConfig.GeminiModel)
	if primaryModel == "" {
		primaryModel = "gemini-3.5-flash-lite"
	}

	// Model candidates in priority order
	candidates := []string{primaryModel}
	fallbacks := []string{"gemini-3.5-flash-lite", "gemini-flash-lite-latest", "gemini-3.8-flash"}
	for _, fb := range fallbacks {
		if fb != primaryModel {
			candidates = append(candidates, fb)
		}
	}

	var lastErr error
	client := &http.Client{Timeout: 30 * time.Second}

	for _, model := range candidates {
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

		reqBody := geminiRequest{
			Contents: []geminiContent{
				{
					Parts: []geminiPart{
						{Text: prompt},
					},
				},
			},
			GenerationConfig: geminiGenerationConfig{
				Temperature:      0.2,
				ResponseMimeType: "application/json",
			},
		}

		jsonBytes, err := json.Marshal(reqBody)
		if err != nil {
			return "", err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("gemini http request failed (model=%s): %w", model, err)
			log.Printf("[Translator] %v; attempting next model...", lastErr)
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()

		if err != nil {
			lastErr = fmt.Errorf("failed to read gemini response (model=%s): %w", model, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("gemini returned HTTP %d (model=%s): %s", resp.StatusCode, model, string(respBytes))
			log.Printf("[Translator] %v; attempting next model...", lastErr)
			continue
		}

		var gResp geminiResponse
		if err := json.Unmarshal(respBytes, &gResp); err != nil {
			lastErr = fmt.Errorf("failed to decode gemini response (model=%s): %w (raw: %s)", model, err, string(respBytes))
			continue
		}

		if gResp.Error != nil && gResp.Error.Message != "" {
			lastErr = fmt.Errorf("gemini api error (model=%s): %s", model, gResp.Error.Message)
			log.Printf("[Translator] %v; attempting next model...", lastErr)
			continue
		}

		if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
			lastErr = fmt.Errorf("gemini returned no content candidates (model=%s)", model)
			continue
		}

		return gResp.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("all gemini translation models failed, last error: %w", lastErr)
}

// TranslateDevotional checks the cache first, or calls Gemini to translate into targetLang.
func TranslateDevotional(devo *models.Devotional, targetLang string) (*models.Devotional, error) {
	if devo == nil {
		return nil, errors.New("devotional is nil")
	}

	cleanLang := strings.ToLower(strings.TrimSpace(targetLang))
	if cleanLang == "" || cleanLang == "en" {
		return devo, nil
	}

	// 1. Check PostgreSQL Translation Cache
	var cached models.ContentTranslation
	err := db.DB.Where(
		"content_type = ? AND content_id = ? AND language = ?",
		"devotional", devo.ID.String(), cleanLang,
	).First(&cached).Error

	if err == nil && cached.TranslatedData != "" {
		var payload DevotionalTranslationPayload
		if jsonErr := json.Unmarshal([]byte(cached.TranslatedData), &payload); jsonErr == nil {
			result := *devo
			if payload.Title != "" {
				result.Title = payload.Title
			}
			if payload.ScriptureQuote != "" {
				result.ScriptureQuote = payload.ScriptureQuote
			}
			if payload.ScriptureReference != "" {
				result.ScriptureReference = payload.ScriptureReference
			}
			if payload.Body != "" {
				result.Body = payload.Body
			}
			if payload.Prayer != "" {
				result.Prayer = payload.Prayer
			}
			if payload.Reflection != "" {
				result.Reflection = payload.Reflection
			}
			return &result, nil
		}
	}

	// 2. If no Gemini API key configured, gracefully return English
	if strings.TrimSpace(config.AppConfig.GeminiAPIKey) == "" {
		return devo, nil
	}

	// 3. Request Translation from Gemini
	langName := GetLanguageName(cleanLang)
	inputData := DevotionalTranslationPayload{
		Title:              devo.Title,
		ScriptureQuote:     devo.ScriptureQuote,
		ScriptureReference: devo.ScriptureReference,
		Body:               devo.Body,
		Prayer:             devo.Prayer,
		Reflection:         devo.Reflection,
	}

	inputBytes, _ := json.Marshal(inputData)

	prompt := fmt.Sprintf(
		"You are a professional Christian theologian and translator. Translate the following devotional JSON into %s.\n"+
			"Guidelines:\n"+
			"- Maintain reverent, sacred biblical language, spiritual depth, and emotional warmth.\n"+
			"- Retain scripture references (e.g., 'John 3:16') in standard %s Bible format.\n"+
			"- Preserve all paragraph breaks and markdown formatting in the body.\n"+
			"- Return a JSON object with the exact keys: 'title', 'scripture_quote', 'scripture_reference', 'body', 'prayer', 'reflection'.\n\n"+
			"Input JSON:\n%s",
		langName, langName, string(inputBytes),
	)

	jsonStr, geminiErr := CallGeminiJSON(prompt)
	if geminiErr != nil {
		log.Printf("[Translator] Gemini devotional translation failed for lang=%s id=%s: %v\n", cleanLang, devo.ID, geminiErr)
		return devo, nil // Fallback to original
	}

	var payload DevotionalTranslationPayload
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		log.Printf("[Translator] Failed to parse Gemini devotional JSON: %v\n", err)
		return devo, nil
	}

	// 4. Save to Database Cache
	savePayload, _ := json.Marshal(payload)
	transRecord := models.ContentTranslation{
		ContentType:    "devotional",
		ContentID:      devo.ID.String(),
		Language:       cleanLang,
		TranslatedData: string(savePayload),
	}
	db.DB.Create(&transRecord)

	// 5. Construct translated response
	result := *devo
	if payload.Title != "" {
		result.Title = payload.Title
	}
	if payload.ScriptureQuote != "" {
		result.ScriptureQuote = payload.ScriptureQuote
	}
	if payload.ScriptureReference != "" {
		result.ScriptureReference = payload.ScriptureReference
	}
	if payload.Body != "" {
		result.Body = payload.Body
	}
	if payload.Prayer != "" {
		result.Prayer = payload.Prayer
	}
	if payload.Reflection != "" {
		result.Reflection = payload.Reflection
	}

	return &result, nil
}

// TranslateHymn checks cache, or calls Gemini to translate hymn lyrics into targetLang.
func TranslateHymn(hymn *models.Hymn, targetLang string) (*models.Hymn, error) {
	if hymn == nil {
		return nil, errors.New("hymn is nil")
	}

	cleanLang := strings.ToLower(strings.TrimSpace(targetLang))
	if cleanLang == "" || cleanLang == "en" {
		return hymn, nil
	}

	contentID := strconv.Itoa(hymn.Number)

	// 1. Check PostgreSQL Translation Cache
	var cached models.ContentTranslation
	err := db.DB.Where(
		"content_type = ? AND content_id = ? AND language = ?",
		"hymn", contentID, cleanLang,
	).First(&cached).Error

	if err == nil && cached.TranslatedData != "" {
		var payload HymnTranslationPayload
		if jsonErr := json.Unmarshal([]byte(cached.TranslatedData), &payload); jsonErr == nil {
			result := *hymn
			if payload.Title != "" {
				result.Title = payload.Title
			}
			if payload.Chorus != "" {
				result.Chorus = payload.Chorus
			}
			if len(payload.Verses) > 0 {
				vBytes, _ := json.Marshal(payload.Verses)
				result.Verses = string(vBytes)
			}
			return &result, nil
		}
	}

	// 2. If no Gemini API key configured, gracefully return English
	if strings.TrimSpace(config.AppConfig.GeminiAPIKey) == "" {
		return hymn, nil
	}

	// 3. Request Translation from Gemini
	langName := GetLanguageName(cleanLang)

	// Unmarshal existing verses
	var versesList []string
	_ = json.Unmarshal([]byte(hymn.Verses), &versesList)

	inputData := HymnTranslationPayload{
		Title:  hymn.Title,
		Chorus: hymn.Chorus,
		Verses: versesList,
	}

	inputBytes, _ := json.Marshal(inputData)

	prompt := fmt.Sprintf(
		"You are a skilled Christian hymn translator. Translate the following hymn into %s.\n"+
			"Guidelines:\n"+
			"- Preserve the musical meter, poetic rhythm, and sacred theological tone.\n"+
			"- Keep each stanza as a separate string element in the 'verses' array, preserving line breaks (\\n) within stanzas.\n"+
			"- Do NOT add stanza numbers (like '1.' or '2.') to the verse lyrics.\n"+
			"- Return a JSON object with the exact keys: 'title', 'chorus', and 'verses' (array of strings).\n\n"+
			"Input JSON:\n%s",
		langName, string(inputBytes),
	)

	jsonStr, geminiErr := CallGeminiJSON(prompt)
	if geminiErr != nil {
		log.Printf("[Translator] Gemini hymn translation failed for lang=%s number=%d: %v\n", cleanLang, hymn.Number, geminiErr)
		return hymn, nil
	}

	var payload HymnTranslationPayload
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		log.Printf("[Translator] Failed to parse Gemini hymn JSON: %v\n", err)
		return hymn, nil
	}

	// 4. Save to Database Cache
	savePayload, _ := json.Marshal(payload)
	transRecord := models.ContentTranslation{
		ContentType:    "hymn",
		ContentID:      contentID,
		Language:       cleanLang,
		TranslatedData: string(savePayload),
	}
	db.DB.Create(&transRecord)

	// 5. Construct translated response
	result := *hymn
	if payload.Title != "" {
		result.Title = payload.Title
	}
	if payload.Chorus != "" {
		result.Chorus = payload.Chorus
	}
	if len(payload.Verses) > 0 {
		vBytes, _ := json.Marshal(payload.Verses)
		result.Verses = string(vBytes)
	}

	return &result, nil
}

type BibleChapterTranslationPayload struct {
	Verses []string `json:"verses"`
}

// TranslateBibleChapter checks cache first, or calls Gemini to translate chapter verses into targetLang.
func TranslateBibleChapter(book string, chapter int, verses []string, targetLang string) ([]string, error) {
	if len(verses) == 0 {
		return verses, nil
	}

	cleanLang := strings.ToLower(strings.TrimSpace(targetLang))
	if cleanLang == "" || cleanLang == "en" {
		return verses, nil
	}

	contentID := fmt.Sprintf("%s_%d", strings.ToLower(strings.TrimSpace(book)), chapter)

	// 1. Check PostgreSQL Translation Cache
	var cached models.ContentTranslation
	err := db.DB.Where(
		"content_type = ? AND content_id = ? AND language = ?",
		"bible", contentID, cleanLang,
	).First(&cached).Error

	if err == nil && cached.TranslatedData != "" {
		var payload BibleChapterTranslationPayload
		if jsonErr := json.Unmarshal([]byte(cached.TranslatedData), &payload); jsonErr == nil && len(payload.Verses) > 0 {
			return payload.Verses, nil
		}
	}

	// 2. If no Gemini API key configured, gracefully return original English verses
	if strings.TrimSpace(config.AppConfig.GeminiAPIKey) == "" {
		return verses, nil
	}

	// 3. Request Translation from Gemini
	langName := GetLanguageName(cleanLang)
	inputData := BibleChapterTranslationPayload{
		Verses: verses,
	}
	inputBytes, _ := json.Marshal(inputData)

	prompt := fmt.Sprintf(
		"You are a revered biblical scholar and Christian Bible translator. Translate the following King James Bible chapter from the Book of %s, Chapter %d into %s.\n"+
			"Guidelines:\n"+
			"- Use authentic, sacred scriptural vocabulary matching the recognized historical Bible translation in %s (e.g., Bíbélì Mímọ́ for Yoruba, Louis Segond for French, Reina-Valera for Spanish, Baịbụl Nsọ for Igbo, Littafi Mai Tsarki for Hausa, João Ferreira de Almeida for Portuguese, Amharic Holy Bible for Amharic).\n"+
			"- Maintain the exact count and order of verses in the 'verses' array.\n"+
			"- Do NOT add verse numbers to the verse strings.\n"+
			"- Return a JSON object with the exact key 'verses' (array of strings).\n\n"+
			"Input JSON:\n%s",
		book, chapter, langName, langName, string(inputBytes),
	)

	jsonStr, geminiErr := CallGeminiJSON(prompt)
	if geminiErr != nil {
		log.Printf("[Translator] Gemini Bible chapter translation failed for lang=%s book=%s chapter=%d: %v\n", cleanLang, book, chapter, geminiErr)
		return verses, nil
	}

	var payload BibleChapterTranslationPayload
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		log.Printf("[Translator] Failed to parse Gemini Bible JSON: %v\n", err)
		return verses, nil
	}

	if len(payload.Verses) == 0 {
		return verses, nil
	}

	// 4. Save to Database Cache
	savePayload, _ := json.Marshal(payload)
	transRecord := models.ContentTranslation{
		ContentType:    "bible",
		ContentID:      contentID,
		Language:       cleanLang,
		TranslatedData: string(savePayload),
	}
	db.DB.Create(&transRecord)

	return payload.Verses, nil
}
