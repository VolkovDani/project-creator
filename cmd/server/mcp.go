package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"strconv"

	"github.com/VolkovDani/project-creator/internal/assistantmcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func runMCP(ctx context.Context, args []string, getenv func(string) string) error {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	project := flags.String("project", "", "ID of the single project shared with this client")
	sharing := flags.String("sharing", "", "consent to share with a local or cloud client (local|cloud)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected MCP argument")
	}
	port := uint64(8080)
	if addr := getenv("PROJECT_CREATOR_ADDR"); addr != "" {
		_, configuredPort, err := net.SplitHostPort(addr)
		if err != nil {
			return errors.New("MCP requires a valid PROJECT_CREATOR_ADDR with a numeric port")
		}
		port, err = strconv.ParseUint(configuredPort, 10, 16)
		if err != nil || port == 0 {
			return errors.New("MCP requires a valid PROJECT_CREATOR_ADDR with a numeric port")
		}
	}
	server, err := assistantmcp.New(assistantmcp.Config{
		Port: uint16(port), Token: getenv("PROJECT_CREATOR_TOKEN"), ProjectID: *project, Sharing: *sharing,
	})
	if err != nil {
		return err
	}
	if err := server.Run(ctx, &mcp.StdioTransport{MaxLineLength: assistantmcp.MaxMessageBytes}); err != nil && ctx.Err() == nil {
		return fmt.Errorf("MCP connection closed with an error: %w", err)
	}
	return nil
}
