/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pulls

import (
	"time"

	github_util "k8s.io/contrib/github"
	"k8s.io/contrib/mungegithub/config"

	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

type LGTMAfterCommitMunger struct{}

func init() {
	RegisterMungerOrDie(LGTMAfterCommitMunger{})
}

func lastModifiedTime(list []github.RepositoryCommit) *time.Time {
	var lastModified *time.Time
	for ix := range list {
		item := list[ix]
		if lastModified == nil || item.Commit.Committer.Date.After(*lastModified) {
			lastModified = item.Commit.Committer.Date
		}
	}
	return lastModified
}

func lgtmTime(events []github.IssueEvent) *time.Time {
	var lgtmTime *time.Time
	for ix := range events {
		event := &events[ix]
		if *event.Event == "labeled" && *event.Label.Name == "lgtm" {
			if lgtmTime == nil || event.CreatedAt.After(*lgtmTime) {
				lgtmTime = event.CreatedAt
			}
		}
	}
	return lgtmTime
}

func (LGTMAfterCommitMunger) Name() string { return "lgtm-after-commit" }

func (LGTMAfterCommitMunger) AddFlags(cmd *cobra.Command) {}

func (LGTMAfterCommitMunger) MungePullRequest(config *config.MungeConfig, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent) {
	lastModified := lastModifiedTime(commits)
	lgtmTime := lgtmTime(events)

	if lastModified == nil || lgtmTime == nil {
		return
	}

	if !github_util.HasLabel(issue.Labels, "lgtm") {
		return
	}

	if lastModified.After(*lgtmTime) {
		lgtmRemovedBody := "PR changed after LGTM, removing LGTM."
		if err := config.WriteComment(*pr.Number, lgtmRemovedBody); err != nil {
			return
		}
		config.RemoveLabel(*pr.Number, "lgtm")
	}
}
