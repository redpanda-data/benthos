// Copyright 2025 Redpanda Data, Inc.

package studio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/config/schema"
)

func syncSchemaCommand(cliOpts *common.CLIOpts) *cli.Command {
	return &cli.Command{
		Name:  "sync-schema",
		Usage: "Synchronizes the schema of this Redpanda Connect instance with a studio session",
		Description: `
This sync allows custom plugins and templates to be configured and linted
correctly within Redpanda Connect studio.

In order to synchronize a single use token must be generated from the session
page within the studio application.`[1:],
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "session",
				Aliases:  []string{"s"},
				Required: true,
				Value:    "",
				Usage:    "The session ID to synchronize with.",
			},
			&cli.StringFlag{
				Name:     "token",
				Aliases:  []string{"t"},
				Required: true,
				Value:    "",
				Usage:    "The single use token used to authenticate the request.",
			},
		},
		Action: func(c *cli.Context) error {
			endpoint := c.String("endpoint")
			sessionID := c.String("session")
			tokenID := c.String("token")

			u, err := url.Parse(endpoint)
			if err != nil {
				return fmt.Errorf("failed to parse endpoint: %w", err)
			}
			u.Path = path.Join(u.Path, fmt.Sprintf("/api/v1/token/%v/session/%v/schema", tokenID, sessionID))

			schema := schema.New(cliOpts.Version, cliOpts.DateBuilt, cliOpts.Environment, cliOpts.BloblEnvironment)
			schema.Config = cliOpts.MainConfigSpecCtor()
			schema.Scrub()
			schemaBytes, err := json.Marshal(schema)
			if err != nil {
				return fmt.Errorf("failed to encode schema: %w", err)
			}

			res, err := http.Post(u.String(), "application/json", bytes.NewReader(schemaBytes))
			if err != nil {
				return fmt.Errorf("sync request failed: %w", err)
			}

			defer res.Body.Close()

			if res.StatusCode < 200 || res.StatusCode > 299 {
				resBytes, _ := io.ReadAll(res.Body)
				return fmt.Errorf("sync request failed (%v): %v", res.StatusCode, string(resBytes))
			}
			return nil
		},
	}
}
