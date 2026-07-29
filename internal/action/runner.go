// Package action wires the inputs, message resolution and MAX client into a
// single run of the GitHub Action.
package action

import (
	"context"
	"io"
	"strconv"

	"github.com/Fgeeha/max-action/internal/config"
	"github.com/Fgeeha/max-action/internal/github"
	"github.com/Fgeeha/max-action/internal/maxapi"
	"github.com/Fgeeha/max-action/internal/message"
)

// Env is everything the runner touches outside of itself.
type Env struct {
	Getenv  func(string) string
	Stdout  io.Writer
	Version string
}

// Run executes one send. Every failure is annotated for the workflow log
// before it is returned, so the caller only has to pick an exit code.
func Run(ctx context.Context, env Env) error {
	log := &github.Logger{Out: env.Stdout}

	// Mask the raw token before any other code can put it in a log line.
	rawToken := env.Getenv("INPUT_TOKEN")
	log.Mask(rawToken)

	cfg, err := config.Load(env.Getenv)
	if err != nil {
		log.Errorf("%s", err)
		return err
	}
	log.Debug = cfg.Debug
	// Inputs arrive trimmed, so mask the trimmed form too when it differs.
	if cfg.Token != rawToken {
		log.Mask(cfg.Token)
	}

	ghCtx := message.ContextFromEnv(env.Getenv)
	text, err := message.Resolve(cfg, ghCtx)
	if err != nil {
		log.Errorf("%s", err)
		return err
	}

	if cfg.APIBaseURL.String() != config.DefaultAPIBaseURL {
		log.Debugf("using a non-default MAX API host: %s", cfg.APIBaseURL.Hostname())
	}

	client, err := maxapi.New(maxapi.Options{
		BaseURL: cfg.APIBaseURL,
		Token:   cfg.Token,
		Timeout: cfg.Timeout,
		Retries: cfg.Retries,
		Version: env.Version,
		Debugf:  log.Debugf,
		Warnf:   log.Warningf,
	})
	if err != nil {
		log.Errorf("%s", err)
		return err
	}

	mid, err := client.Send(ctx, maxapi.SendMessage{
		RecipientType:      cfg.RecipientType,
		RecipientID:        cfg.RecipientID,
		Text:               text,
		Format:             cfg.Format,
		Notify:             cfg.Notify,
		DisableLinkPreview: cfg.DisableLinkPreview,
	})
	if err != nil {
		log.Errorf("%s", err)
		return err
	}

	// The message is already delivered at this point. A failure to record the
	// outputs must not fail the step, or a retry would send it twice.
	recipientID := strconv.FormatInt(cfg.RecipientID, 10)
	if err := github.WriteOutputs(env.Getenv("GITHUB_OUTPUT"), []github.Output{
		{Name: "message_id", Value: mid},
		{Name: "recipient_type", Value: cfg.RecipientType},
		{Name: "recipient_id", Value: recipientID},
	}); err != nil {
		log.Warningf("message sent, but the outputs could not be recorded: %s", err)
	}

	log.Printf("MAX message sent successfully to %s %s", cfg.RecipientType, recipientID)
	return nil
}
