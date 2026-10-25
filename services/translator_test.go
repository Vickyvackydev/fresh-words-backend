package services

import (
	"encoding/json"
	"testing"

	"fresh-words-backend/config"
	"fresh-words-backend/models"

	"github.com/joho/godotenv"
)

func TestYorubaDevotionalTranslation(t *testing.T) {
	_ = godotenv.Load("../.env")
	config.LoadConfig()
	if config.AppConfig.GroqAPIKey == "" {
		t.Skip("GROQ_API_KEY is not set, skipping live translation test")
	}

	devo := &models.Devotional{
		Title:              "Walking in Divine Faith",
		ScriptureReference: "Proverbs 3:5-6",
		ScriptureQuote:     "Trust in the Lord with all your heart, and do not lean on your own understanding.",
		Body:               "Every single day presents us with choices. When uncertainties surround you, remember that God's wisdom far exceeds human intellect. Surrender your plans to Him today and let His Holy Spirit guide your steps.",
		Prayer:             "Heavenly Father, grant me the grace to trust Your promises each day. In Jesus' name, Amen.",
		Reflection:         "In what area of your life are you struggling to trust God today?",
	}

	draft, err := GenerateAIDevotionalDraft(devo, "yo")
	if err != nil {
		t.Fatalf("GenerateAIDevotionalDraft failed: %v", err)
	}

	if draft == nil {
		t.Fatal("Expected draft, got nil")
	}

	b, _ := json.MarshalIndent(draft, "", "  ")
	t.Logf("Translated Yoruba Devotional:\n%s\n", string(b))

	if draft.Title == "" || draft.Body == "" || draft.Prayer == "" {
		t.Errorf("Draft has empty required fields: %+v", draft)
	}
}
