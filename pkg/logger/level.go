package logger

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	PanicLevel
	FatalLevel
)

func (l Level) Parse(txt string) (Level, error) {
	var level Level

	switch strings.ToLower(string(txt)) {
	case "debug":
		level = DebugLevel
	case "info", "":
		level = InfoLevel
	case "warn":
		level = WarnLevel
	case "error":
		level = ErrorLevel
	case "panic":
		level = PanicLevel
	case "fatal":
		level = FatalLevel
	default:
		return 0, fmt.Errorf("unknown log level: %v", string(txt))
	}

	return level, nil
}

func (l Level) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

func (l *Level) UnmarshalText(text []byte) error {
	if l == nil {
		return fmt.Errorf("empty string")
	}

	val, err := l.Parse(string(text))
	if err != nil {
		return err
	}

	*l = val

	return nil
}

func (l Level) String() string {
	text, ok := l.All()[l]
	if ok {
		return text
	} else {
		return fmt.Sprintf("Level(%d)", l)
	}
}

func (l Level) All() map[Level]string {
	return map[Level]string{
		DebugLevel: "debug",
		InfoLevel:  "info",
		WarnLevel:  "warn",
		ErrorLevel: "error",
		PanicLevel: "panic",
		FatalLevel: "fatal",
	}
}

func (l Level) Names() []string {
	names := []string{}
	all := l.All()

	keys := slices.Collect(maps.Keys(all))
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	for _, k := range keys {
		item := all[k]
		names = append(names, item)
	}

	return names
}
