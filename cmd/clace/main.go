// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/claceio/clace/internal/system"
	"github.com/claceio/clace/internal/types"
)

// Added by goreleaser as build information
var (
	gitCommit  string // gitCommit is the git commit that was compiled
	gitVersion string // gitVersion is the build tag
)

const configFileFlagName = "config-file"

func getAllCommands(clientConfig *types.ClientConfig, serverConfig *types.ServerConfig) ([]*cli.Command, error) {
	var allCommands []*cli.Command
	serverCommands, err := getServerCommands(serverConfig, clientConfig)
	if err != nil {
		return nil, err
	}

	clientCommands, err := getClientCommands(clientConfig)
	if err != nil {
		return nil, err
	}

	passwordCommands, err := getPasswordCommands(clientConfig)
	if err != nil {
		return nil, err
	}

	for _, v := range [][]*cli.Command{
		serverCommands,
		clientCommands,
		passwordCommands,
	} {
		allCommands = append(allCommands, v...)
	}
	return allCommands, nil
}

func globalFlags(globalConfig *types.GlobalConfig) ([]cli.Flag, error) {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        configFileFlagName,
			Aliases:     []string{"c"},
			Usage:       "TOML configuration file",
			Destination: &globalConfig.ConfigFile,
			EnvVars:     []string{"CL_CONFIG_FILE"},
		},
		&cli.BoolFlag{
			Name:    "version",
			Aliases: []string{"v"},
			Usage:   "Print version info",
		},
	}, nil
}

// getConfigPath returns the path to the config file and the home directory
// Uses CL_HOME env if set. Otherwise uses binaries parent path. Setting CL_HOME is
// the easiest way to configure. Uses some extra heuristics to help avoid having to setup
// CL_HOME in the env, by using the binaries parent folder as the default.
// On mac, looks for brew install locations also.
func getConfigPath(cCtx *cli.Context) (string, string, error) {
	configFile := cCtx.String(configFileFlagName)
	clHome := os.Getenv(types.CL_HOME)
	if configFile == "" {
		configFile = os.Getenv("CL_CONFIG_FILE")
		if configFile == "" && clHome != "" {
			configFile = path.Join(clHome, "clace.toml")
		}
	}
	if clHome != "" {
		// Found CL_HOME
		return clHome, configFile, nil
	}
	if configFile != "" {
		// CL_HOME not set and config file is set, use config dir path as CL_HOME
		clHome = filepath.Dir(configFile)
		return clHome, configFile, nil
	}

	binFile, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("unable to find executable path: %w", err)
	}
	binAbsolute, err := filepath.EvalSymlinks(binFile)
	if err != nil {
		return "", "", fmt.Errorf("unable to resolve symlink: %w", err)
	}

	binParent := filepath.Dir(binAbsolute)
	if filepath.Base(binParent) == "bin" {
		// Found bin directory, use its parent
		binParent = filepath.Dir(binParent)
	}
	binParentConfig := path.Join(binParent, "clace.toml")
	if system.FileExists(binParentConfig) && (strings.Contains(binParent, "clace") || strings.Contains(binParent, "clhome")) {
		// Config file found in parent directory of the executable, use that as path
		// To avoid clobbering /usr, check if the path contains the string clace/clhome
		return binParent, binParentConfig, nil
	}

	// Running `brew --prefix` would be another option
	if runtime.GOOS == "darwin" {
		// brew OSX specific checks
		if system.FileExists("/opt/homebrew/etc/clace.toml") {
			return "/opt/homebrew/var/clace", "/opt/homebrew/etc/clace.toml", nil
		} else if system.FileExists("/usr/local/etc/clace.toml") {
			return "/usr/local/var/clace", "/usr/local/etc/clace.toml", nil
		}
	} else if runtime.GOOS == "linux" {
		// brew linux specific checks
		if system.FileExists("/home/linuxbrew/.linuxbrew/etc/clace.toml") {
			return "/home/linuxbrew/.linuxbrew/var/clace", "/home/linuxbrew/.linuxbrew/etc/clace.toml", nil
		} else if system.FileExists("/usr/local/etc/clace.toml") {
			return "/usr/local/var/clace", "/usr/local/etc/clace.toml", nil
		}
	}
	return "", "", fmt.Errorf("unable to find CL_HOME or config file")
}

func parseConfig(cCtx *cli.Context, globalConfig *types.GlobalConfig, clientConfig *types.ClientConfig, serverConfig *types.ServerConfig) error {
	// Find CL_HOME and config file, update CL_HOME in env
	clHome, filePath, err := getConfigPath(cCtx)
	if err != nil {
		return err
	}
	clHome, err = filepath.Abs(clHome)
	if err != nil {
		return fmt.Errorf("unable to resolve CL_HOME: %w", err)
	}
	os.Setenv(types.CL_HOME, clHome)

	//fmt.Fprintf(os.Stderr, "Loading config file: %s, clHome %s\n", filePath, clHome)
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	if err := system.LoadGlobalConfig(string(buf), globalConfig); err != nil {
		return err
	}
	if err := system.LoadClientConfig(string(buf), clientConfig); err != nil {
		return err
	}
	if err := system.LoadServerConfig(string(buf), serverConfig); err != nil {
		return err
	}

	return nil
}

func main() {
	globalConfig, clientConfig, serverConfig, err := system.GetDefaultConfigs()
	if err != nil {
		log.Fatal(err)
	}
	globalFlags, err := globalFlags(globalConfig)
	if err != nil {
		log.Fatal(err)
	}
	allCommands, err := getAllCommands(clientConfig, serverConfig)
	if err != nil {
		log.Fatal(err)
	}

	app := &cli.App{
		Name:                 "clace",
		Usage:                "Clace client and server https://clace.io/",
		EnableBashCompletion: true,
		Suggest:              true,
		Flags:                globalFlags,
		Before: func(ctx *cli.Context) error {
			err := parseConfig(ctx, globalConfig, clientConfig, serverConfig)
			if ctx.Command != nil && ctx.Args().Len() > 0 && ctx.Args().Get(0) == "password" {
				// For password command, ignore error parsing config
				return nil
			}
			if err != nil {
				return fmt.Errorf("error parsing config: %w", err)
			}
			return nil
		},
		ExitErrHandler: func(c *cli.Context, err error) {
			if err != nil {
				fmt.Fprintf(cli.ErrWriter, RED+"error: %s\n"+RESET, err)
				os.Exit(1)
			}
		},
		Commands: allCommands,
		Action: func(ctx *cli.Context) error {
			// Default action when no subcommand is specified
			if ctx.Bool("version") {
				printVersion(ctx)
				os.Exit(0)
			}
			return cli.ShowAppHelp(ctx)
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		os.Exit(1)
	}
}
