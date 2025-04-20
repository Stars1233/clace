// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"testing"

	"github.com/claceio/clace/internal/testutil"
)

func TestParseGithub(t *testing.T) {
	tests := map[string]struct {
		url        string
		wantRepo   string
		wantFolder string
		wantError  error
	}{
		"blank":                    {url: "", wantRepo: "", wantFolder: "", wantError: fmt.Errorf("invalid github url: https:///, expected github.com/orgName/repoName or github.com/orgName/repoName/folder")},
		"org and repo only":        {url: "github.com/orgName/repoName", wantRepo: "https://github.com/orgName/repoName", wantFolder: "", wantError: nil},
		"org, repo and folder":     {url: "http://github.com/orgName/repoName/folderName", wantRepo: "http://github.com/orgName/repoName", wantFolder: "folderName/", wantError: nil},
		"org, repo and subfolders": {url: "https://github.com/orgName/repoName/folderName/sub", wantRepo: "https://github.com/orgName/repoName", wantFolder: "folderName/sub/", wantError: nil},
		"giturl":                   {url: "git@github.com:user/repo.git", wantRepo: "git@github.com:user/repo.git", wantFolder: "", wantError: nil},
		"giturl and folder":        {url: "git@github.com:user/repo.git/folderName", wantRepo: "git@github.com:user/repo.git", wantFolder: "folderName/", wantError: nil},
		"giturl and subfolders":    {url: "git@github.com:user/repo.git/folderName/sub", wantRepo: "git@github.com:user/repo.git", wantFolder: "folderName/sub/", wantError: nil},
		"invalid giturl":           {url: "git@github.com:user", wantRepo: "", wantFolder: "", wantError: fmt.Errorf("invalid github url: git@github.com:user/, expected git@github.com:orgName/repoName or git@github.com:orgName/repoName/folder")},
		"invalid url":              {url: "/orgName", wantRepo: "", wantFolder: "", wantError: fmt.Errorf("invalid github url: https:///orgName/, expected github.com/orgName/repoName or github.com/orgName/repoName/folder")},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotRepo, gotFolder, gotError := parseGithubUrl(tc.url, "")
			testutil.AssertEqualsString(t, "repo", gotRepo, tc.wantRepo)
			testutil.AssertEqualsString(t, "folder", gotFolder, tc.wantFolder)
			testutil.AssertEqualsError(t, "error", gotError, tc.wantError)
		})
	}
}

func TestParseGithubAuth(t *testing.T) {
	tests := map[string]struct {
		url        string
		wantRepo   string
		wantFolder string
		wantError  error
	}{
		"blank":                    {url: "", wantRepo: "", wantFolder: "", wantError: fmt.Errorf("invalid github url: https:///, expected github.com/orgName/repoName or github.com/orgName/repoName/folder")},
		"org and repo only":        {url: "github.com/orgName/repoName", wantRepo: "git@github.com:orgName/repoName.git", wantFolder: "", wantError: nil},
		"org, repo and folder":     {url: "http://github.com/orgName/repoName/folderName", wantRepo: "git@github.com:orgName/repoName.git", wantFolder: "folderName/", wantError: nil},
		"org, repo and subfolders": {url: "https://github.com/orgName/repoName/folderName/sub", wantRepo: "git@github.com:orgName/repoName.git", wantFolder: "folderName/sub/", wantError: nil},
		"giturl":                   {url: "git@github.com:user/repo.git", wantRepo: "git@github.com:user/repo.git", wantFolder: "", wantError: nil},
		"giturl and folder":        {url: "git@github.com:user/repo.git/folderName", wantRepo: "git@github.com:user/repo.git", wantFolder: "folderName/", wantError: nil},
		"giturl and subfolders":    {url: "git@github.com:user/repo.git/folderName/sub", wantRepo: "git@github.com:user/repo.git", wantFolder: "folderName/sub/", wantError: nil},
		"invalid giturl":           {url: "git@github.com:user", wantRepo: "", wantFolder: "", wantError: fmt.Errorf("invalid github url: git@github.com:user/, expected git@github.com:orgName/repoName or git@github.com:orgName/repoName/folder")},
		"invalid url":              {url: "/orgName", wantRepo: "", wantFolder: "", wantError: fmt.Errorf("invalid github url: https:///orgName/, expected github.com/orgName/repoName or github.com/orgName/repoName/folder")},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotRepo, gotFolder, gotError := parseGithubUrl(tc.url, "authkey")
			testutil.AssertEqualsString(t, "repo", gotRepo, tc.wantRepo)
			testutil.AssertEqualsString(t, "folder", gotFolder, tc.wantFolder)
			testutil.AssertEqualsError(t, "error", gotError, tc.wantError)
		})
	}
}
