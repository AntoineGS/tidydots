package config

import (
	"log/slog"
	"strings"
)

// EvaluateWhen evaluates a template-based when expression without logging.
// Empty when returns true (always match). Nil renderer returns false.
// The template is rendered and the trimmed result is checked against "true".
// Any render error results in false (silent — prefer EvaluateWhenWithLogger
// when a logger is in scope so typos in when expressions are not swallowed).
func EvaluateWhen(when string, renderer PathRenderer) bool {
	return EvaluateWhenWithLogger(when, renderer, nil)
}

// EvaluateWhenWithLogger is like EvaluateWhen but logs template render errors
// at warn level to the supplied logger. A nil logger is equivalent to
// EvaluateWhen (errors are swallowed).
func EvaluateWhenWithLogger(when string, renderer PathRenderer, logger *slog.Logger) bool {
	if strings.TrimSpace(when) == "" {
		return true
	}

	if renderer == nil {
		return false
	}

	result, err := renderer.RenderString("when", when)
	if err != nil {
		if logger != nil {
			logger.Warn("when expression failed to render; entry excluded",
				slog.String("when", when),
				slog.String("error", err.Error()))
		}
		return false
	}

	return strings.TrimSpace(result) == "true"
}
