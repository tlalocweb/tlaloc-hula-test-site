package csvlog

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

type Entry struct {
	IP                string
	BrowserID         string
	Name              string
	Email             string
	Message           string
	FinalLabel        string
	RealInterestScore int
	SpamScore         int
	SalesPitchScore   int
	Confidence        int
	ReasonShort       string
	UsedModel         bool
}

var header = []string{
	"timestamp", "ip", "browser_id", "name", "email", "message",
	"final_label", "real_interest_score", "spam_score", "sales_pitch_score",
	"confidence", "reason_short", "used_model",
}

type Logger struct {
	mu   sync.Mutex
	file *os.File
	w    *csv.Writer
}

func New(filePath string) (*Logger, error) {
	isNew := false
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		isNew = true
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening csv log: %w", err)
	}

	w := csv.NewWriter(f)

	if isNew {
		if err := w.Write(header); err != nil {
			f.Close()
			return nil, fmt.Errorf("writing csv header: %w", err)
		}
		w.Flush()
	}

	return &Logger{file: f, w: w}, nil
}

func (l *Logger) Log(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	row := []string{
		time.Now().UTC().Format(time.RFC3339),
		e.IP,
		e.BrowserID,
		e.Name,
		e.Email,
		e.Message,
		e.FinalLabel,
		strconv.Itoa(e.RealInterestScore),
		strconv.Itoa(e.SpamScore),
		strconv.Itoa(e.SalesPitchScore),
		strconv.Itoa(e.Confidence),
		e.ReasonShort,
		strconv.FormatBool(e.UsedModel),
	}

	if err := l.w.Write(row); err != nil {
		return fmt.Errorf("writing csv row: %w", err)
	}
	l.w.Flush()
	return l.w.Error()
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.w.Flush()
	return l.file.Close()
}
