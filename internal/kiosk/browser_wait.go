package kiosk

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"
)

func waitForDisplay(ctx context.Context, display string, wakeCmds [][]string) error {
	if len(wakeCmds) == 0 {
		return nil
	}
	for {
		if err := runDisplayCommands(display, wakeCmds); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func waitForURL(ctx context.Context, rawURL string, tlsCfg *tls.Config) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode < 500 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
