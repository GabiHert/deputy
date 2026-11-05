package domain

// Language is a spoken language for voice note transcription.
type Language string

const (
	LangPTBR Language = "pt-BR"
	LangEN   Language = "en"
)

// Languages lists the languages Deputy can transcribe, in display order.
var Languages = []Language{LangPTBR, LangEN}

// Label is the language's display name.
func (l Language) Label() string {
	switch l {
	case LangPTBR:
		return "Portuguese (Brazil)"
	case LangEN:
		return "English"
	}
	return string(l)
}
