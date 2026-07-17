package notification

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	maxIDLength       = 160
	maxTitleLength    = 120
	maxSubtitleLength = 160
	maxBodyLength     = 512
)

type Message struct {
	ID       string
	Title    string
	Subtitle string
	Body     string
}

func (message Message) Validate() error {
	if err := validateText("id", message.ID, maxIDLength, false); err != nil {
		return err
	}
	if err := validateText("title", message.Title, maxTitleLength, false); err != nil {
		return err
	}
	if err := validateText("subtitle", message.Subtitle, maxSubtitleLength, true); err != nil {
		return err
	}
	return validateText("body", message.Body, maxBodyLength, true)
}

func validateText(field, value string, limit int, optional bool) error {
	if strings.TrimSpace(value) == "" && !optional {
		return fmt.Errorf("notification %s is required", field)
	}
	if len(value) > limit {
		return fmt.Errorf("notification %s exceeds %d bytes", field, limit)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("notification %s contains an unsafe control character", field)
		}
	}
	return nil
}
