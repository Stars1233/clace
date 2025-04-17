// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/claceio/clace/internal/system"
	"github.com/claceio/clace/internal/types"
	"github.com/claceio/clace/pkg/api"
	"github.com/pkg/profile"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
)

func getServerCommands(serverConfig *types.ServerConfig, clientConfig *types.ClientConfig) ([]*cli.Command, error) {
	flags := []cli.Flag{}
	return []*cli.Command{
		{
			Name:  "server",
			Usage: "Manage the Clace server",
			Subcommands: []*cli.Command{
				{
					Name:   "start",
					Usage:  "Start the clace server",
					Flags:  flags,
					Before: altsrc.InitInputSourceWithContext(flags, altsrc.NewTomlSourceFromFlagFunc(configFileFlagName)),
					Action: func(cCtx *cli.Context) error {
						return startServer(cCtx, serverConfig)
					},
				},
				{
					Name:   "stop",
					Usage:  "Stop the clace server",
					Flags:  flags,
					Before: altsrc.InitInputSourceWithContext(flags, altsrc.NewTomlSourceFromFlagFunc(configFileFlagName)),
					Action: func(cCtx *cli.Context) error {
						return stopServer(cCtx, clientConfig)
					},
				},
			},
		},
	}, nil
}

func startServer(cCtx *cli.Context, serverConfig *types.ServerConfig) error {
	apiConfig := api.ServerConfig{ServerConfig: serverConfig}
	server, err := api.NewServer(&apiConfig)
	if err != nil {
		fmt.Printf("Error initializing server: %s\n", err)
		os.Exit(1)
	}
	err = server.Start()
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
		os.Exit(1)
	}

	if serverConfig.Http.Port >= 0 {
		addr := fmt.Sprintf("http://%s:%d", serverConfig.Http.Host, serverConfig.Http.Port)
		fmt.Fprintf(os.Stderr, "Server listening on %s\n", addr)
	}
	if serverConfig.Https.Port >= 0 {
		addr := fmt.Sprintf("https://%s:%d", serverConfig.Https.Host, serverConfig.Https.Port)
		fmt.Fprintf(os.Stderr, "Server listening on %s\n", addr)
	}

	clHome := os.ExpandEnv("$CL_HOME")
	switch serverConfig.ProfileMode {
	case "cpu":
		defer profile.Start(profile.CPUProfile, profile.ProfilePath(clHome)).Stop()
	case "memory":
		defer profile.Start(profile.MemProfile, profile.ProfilePath(clHome)).Stop()
	case "allocs":
		defer profile.Start(profile.MemProfileAllocs, profile.ProfilePath(clHome)).Stop()
	case "mutex":
		defer profile.Start(profile.MutexProfile, profile.ProfilePath(clHome)).Stop()
	case "block":
		defer profile.Start(profile.BlockProfile, profile.ProfilePath(clHome)).Stop()
	case "goroutine":
		defer profile.Start(profile.GoroutineProfile, profile.ProfilePath(clHome)).Stop()
	case "clock":
		defer profile.Start(profile.ClockProfile, profile.ProfilePath(clHome)).Stop()
	case "":
		// no profiling
	default:
		fmt.Fprintf(os.Stderr, "Unknown profile mode: %s. Supported modes cpu,memory,allocs,mutex,block,goroutine,clock\n", serverConfig.ProfileMode)
		os.Exit(1)
	}
	if serverConfig.ProfileMode != "" {
		fmt.Fprintf(os.Stderr, "Profiling enabled: %s\n", serverConfig.ProfileMode)
		select {} // block forever, profiling will exit on interrupt
	}

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 30)
	defer cancel()
	_ = server.Stop(ctxTimeout)
	return nil
}

func stopServer(_ *cli.Context, clientConfig *types.ClientConfig) error {
	client := system.NewHttpClient(clientConfig.ServerUri, clientConfig.AdminUser, clientConfig.Client.AdminPassword, clientConfig.Client.SkipCertCheck)

	var response types.AppVersionListResponse
	err := client.Post("/_clace/stop", nil, nil, &response)
	if err == nil {
		return fmt.Errorf("expected error response when stopping server")
	}
	if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
