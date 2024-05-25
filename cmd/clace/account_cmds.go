// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/claceio/clace/internal/system"
	"github.com/claceio/clace/internal/types"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
)

func initAccountCommand(commonFlags []cli.Flag, clientConfig *types.ClientConfig) *cli.Command {
	return &cli.Command{
		Name:  "account",
		Usage: "Manage Clace accounts",
		Subcommands: []*cli.Command{
			accountLinkCommand(commonFlags, clientConfig),
			accountListCommand(commonFlags, clientConfig),
		},
	}
}

func accountLinkCommand(commonFlags []cli.Flag, clientConfig *types.ClientConfig) *cli.Command {
	flags := make([]cli.Flag, 0, len(commonFlags)+2)
	flags = append(flags, commonFlags...)
	flags = append(flags, dryRunFlag())
	flags = append(flags, newBoolFlag(PROMOTE_FLAG, "p", "Promote the change from stage to prod", false))

	return &cli.Command{
		Name:      "link",
		Usage:     "Link an app to to use specific account for a plugin",
		Flags:     flags,
		Before:    altsrc.InitInputSourceWithContext(flags, altsrc.NewTomlSourceFromFlagFunc(configFileFlagName)),
		ArgsUsage: "<pathSpec> <pluginName> <accountName>",
		UsageText: `args: <pathSpec> <pluginName> <accountName>

<pathSpec> is the first required argument. ` + PATH_SPEC_HELP + `<pluginName> is the required second argument. This is the name of the plugin.
<accountName> is the required third argument. This is the name of the account to link to for the plugin. Use "-" to unlink the existing account.

	Examples:
	  Link db plugin: clace account link /myapp store.in temp
	  Link in dryrun mode: clace account link --dry-run example.com:/ rest.in testaccount`,
		Action: func(cCtx *cli.Context) error {
			if cCtx.NArg() != 3 {
				return fmt.Errorf("requires three arguments: <pathSpec> <pluginName> <accountName>")
			}

			client := system.NewHttpClient(clientConfig.ServerUri, clientConfig.AdminUser, clientConfig.Client.AdminPassword, clientConfig.Client.SkipCertCheck)
			values := url.Values{}
			values.Add("pathSpec", cCtx.Args().First())
			values.Add("plugin", cCtx.Args().Get(1))
			values.Add("account", cCtx.Args().Get(2))
			values.Add(DRY_RUN_ARG, strconv.FormatBool(cCtx.Bool(DRY_RUN_FLAG)))
			values.Add(PROMOTE_ARG, strconv.FormatBool(cCtx.Bool(PROMOTE_FLAG)))

			var linkResponse types.AppLinkAccountResponse
			err := client.Post("/_clace/link_account", values, nil, &linkResponse)
			if err != nil {
				return err
			}

			for _, linkedApp := range linkResponse.StagedUpdateResults {
				fmt.Printf("Linked app %s\n", linkedApp)
			}

			if len(linkResponse.PromoteResults) > 0 {
				fmt.Fprintf(cCtx.App.Writer, "Promoted apps: ")
				for i, promoteResult := range linkResponse.PromoteResults {
					if i > 0 {
						fmt.Fprintf(cCtx.App.Writer, ", ")
					}
					fmt.Fprintf(cCtx.App.Writer, "%s", promoteResult)
				}
				fmt.Fprintln(cCtx.App.Writer)
			}

			fmt.Fprintf(cCtx.App.Writer, "%d app(s) linked, %d app(s) promoted.\n", len(linkResponse.StagedUpdateResults), len(linkResponse.PromoteResults))

			if linkResponse.DryRun {
				fmt.Print(DRY_RUN_MESSAGE)
			}

			return nil
		},
	}
}

func accountListCommand(commonFlags []cli.Flag, clientConfig *types.ClientConfig) *cli.Command {
	flags := make([]cli.Flag, 0, len(commonFlags)+2)
	flags = append(flags, commonFlags...)
	flags = append(flags, dryRunFlag())

	return &cli.Command{
		Name:      "list",
		Usage:     "List the accounts linked to an app",
		Flags:     flags,
		Before:    altsrc.InitInputSourceWithContext(flags, altsrc.NewTomlSourceFromFlagFunc(configFileFlagName)),
		ArgsUsage: "<appPath>",
		UsageText: `args: <appPath>

    <app_path> is a required first argument. The optional domain and path are separated by a ":". This is the app for which the accounts are to be listed.

	Examples:
	  List plugins for app: clace account list /myapp
	  List plugins for app: clace account list example.com:/`,
		Action: func(cCtx *cli.Context) error {
			if cCtx.NArg() != 1 {
				return fmt.Errorf("requires one argument: <appPath>")
			}

			client := system.NewHttpClient(clientConfig.ServerUri, clientConfig.AdminUser, clientConfig.Client.AdminPassword, clientConfig.Client.SkipCertCheck)
			values := url.Values{}
			values.Add("appPath", cCtx.Args().First())

			var response types.AppGetResponse
			err := client.Get("/_clace/app", values, &response)
			if err != nil {
				return err
			}

			appInfo := response.AppEntry
			if len(appInfo.Metadata.Accounts) == 0 {
				fmt.Fprintf(cCtx.App.Writer, "No account links for app %s : %s\n", appInfo.AppPathDomain(), appInfo.Id)
				return nil
			}
			fmt.Fprintf(cCtx.App.Writer, "Account links for app %s : %s\n", appInfo.AppPathDomain(), appInfo.Id)
			for _, plugin := range appInfo.Metadata.Accounts {
				fmt.Fprintf(cCtx.App.Writer, "  %s: %s\n", plugin.Plugin, plugin.AccountName)
			}

			return nil
		},
	}
}

func initParamCommand(commonFlags []cli.Flag, clientConfig *types.ClientConfig) *cli.Command {
	return &cli.Command{
		Name:  "param",
		Usage: "Manage app parameter values",
		Subcommands: []*cli.Command{
			updateParamsCommand(commonFlags, clientConfig),
			paramListCommand(commonFlags, clientConfig),
		},
	}
}

func updateParamsCommand(commonFlags []cli.Flag, clientConfig *types.ClientConfig) *cli.Command {
	flags := make([]cli.Flag, 0, len(commonFlags)+2)
	flags = append(flags, commonFlags...)
	flags = append(flags, dryRunFlag())
	flags = append(flags, newBoolFlag(PROMOTE_FLAG, "p", "Promote the change from stage to prod", false))

	return &cli.Command{
		Name:      "update",
		Usage:     "Update parameter value for the app",
		Flags:     flags,
		Before:    altsrc.InitInputSourceWithContext(flags, altsrc.NewTomlSourceFromFlagFunc(configFileFlagName)),
		ArgsUsage: "<paramName> <paramValue> <pathSpec>",
		UsageText: `args: <paramName> <paramValue> <pathSpec>

<paramName> is the first required argument. This is the parameter name.
<paramValue> is the second required argument. This is the value to set the param to. Use "-" to unset the parameter.
<pathSpec> is the third required argument. ` + PATH_SPEC_HELP + `

	Examples:
	  Update parameter value: clace param update port 8888 /myapp
	  Delete parameter value: clace param update port - /myapp`,
		Action: func(cCtx *cli.Context) error {
			if cCtx.NArg() != 3 {
				return fmt.Errorf("requires three arguments: <paramName> <paramValue> <pathSpec>")
			}

			client := system.NewHttpClient(clientConfig.ServerUri, clientConfig.AdminUser, clientConfig.Client.AdminPassword, clientConfig.Client.SkipCertCheck)
			values := url.Values{}
			values.Add("paramName", cCtx.Args().Get(0))
			values.Add("paramValue", cCtx.Args().Get(1))
			values.Add("pathSpec", cCtx.Args().Get(2))
			values.Add(DRY_RUN_ARG, strconv.FormatBool(cCtx.Bool(DRY_RUN_FLAG)))
			values.Add(PROMOTE_ARG, strconv.FormatBool(cCtx.Bool(PROMOTE_FLAG)))

			var updateResponse types.AppLinkAccountResponse
			err := client.Post("/_clace/update_param", values, nil, &updateResponse)
			if err != nil {
				return err
			}

			for _, app := range updateResponse.StagedUpdateResults {
				fmt.Printf("Updated app %s\n", app)
			}

			if len(updateResponse.PromoteResults) > 0 {
				fmt.Fprintf(cCtx.App.Writer, "Promoted apps: ")
				for i, promoteResult := range updateResponse.PromoteResults {
					if i > 0 {
						fmt.Fprintf(cCtx.App.Writer, ", ")
					}
					fmt.Fprintf(cCtx.App.Writer, "%s", promoteResult)
				}
				fmt.Fprintln(cCtx.App.Writer)
			}

			fmt.Fprintf(cCtx.App.Writer, "%d app(s) updated, %d app(s) promoted.\n", len(updateResponse.StagedUpdateResults), len(updateResponse.PromoteResults))

			if updateResponse.DryRun {
				fmt.Print(DRY_RUN_MESSAGE)
			}

			return nil
		},
	}
}

func paramListCommand(commonFlags []cli.Flag, clientConfig *types.ClientConfig) *cli.Command {
	flags := make([]cli.Flag, 0, len(commonFlags)+2)
	flags = append(flags, commonFlags...)
	flags = append(flags, dryRunFlag())

	return &cli.Command{
		Name:      "list",
		Usage:     "List the params for an app",
		Flags:     flags,
		Before:    altsrc.InitInputSourceWithContext(flags, altsrc.NewTomlSourceFromFlagFunc(configFileFlagName)),
		ArgsUsage: "<appPath>",
		UsageText: `args: <appPath>

    <app_path> is a required first argument. The optional domain and path are separated by a ":". This is the app for which the params are to be listed.

	Examples:
	  List params for app: clace param list /myapp
	  List params for app: clace param list example.com:/`,
		Action: func(cCtx *cli.Context) error {
			if cCtx.NArg() != 1 {
				return fmt.Errorf("requires one argument: <appPath>")
			}

			client := system.NewHttpClient(clientConfig.ServerUri, clientConfig.AdminUser, clientConfig.Client.AdminPassword, clientConfig.Client.SkipCertCheck)
			values := url.Values{}
			values.Add("appPath", cCtx.Args().First())

			var response types.AppGetResponse
			err := client.Get("/_clace/app", values, &response)
			if err != nil {
				return err
			}

			appInfo := response.AppEntry
			if len(appInfo.Metadata.ParamValues) == 0 {
				fmt.Fprintf(cCtx.App.Writer, "No param values for app %s : %s\n", appInfo.AppPathDomain(), appInfo.Id)
				return nil
			}
			fmt.Fprintf(cCtx.App.Writer, "Param values for app %s : %s\n", appInfo.AppPathDomain(), appInfo.Id)
			for name, value := range appInfo.Metadata.ParamValues {
				fmt.Fprintf(cCtx.App.Writer, "  %s: %s\n", name, value)
			}

			return nil
		},
	}
}
