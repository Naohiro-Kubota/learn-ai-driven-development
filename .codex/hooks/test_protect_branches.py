#!/usr/bin/env python3
"""Regression tests for the Codex protected-branch hook."""

from __future__ import annotations

import importlib.util
import io
import json
import sys
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch


HOOK_PATH = Path(__file__).with_name("protect_branches.py")
SPEC = importlib.util.spec_from_file_location("protect_branches", HOOK_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("unable to load protect_branches hook")
HOOK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HOOK)


def invoke(command: str, branch: str) -> dict[str, object] | None:
    event = json.dumps({"cwd": "/workspace", "tool_input": {"command": command}})
    output = io.StringIO()
    with (
        patch.object(sys, "stdin", io.StringIO(event)),
        patch.object(HOOK, "current_branch", return_value=branch),
        redirect_stdout(output),
    ):
        HOOK.main()

    content = output.getvalue()
    return json.loads(content) if content else None


def invoke_in_worktree(
    command: str, parent_branch: str, worktree_branch: str
) -> dict[str, object] | None:
    parent_cwd = "/workspace/main"
    worktree_cwd = "/workspace/feature-worktree"
    event = json.dumps(
        {
            "cwd": parent_cwd,
            "tool_input": {"command": command, "workdir": worktree_cwd},
        }
    )
    output = io.StringIO()
    with (
        patch.object(sys, "stdin", io.StringIO(event)),
        patch.object(
            HOOK,
            "current_branch",
            side_effect=lambda cwd: {
                parent_cwd: parent_branch,
                worktree_cwd: worktree_branch,
            }[cwd],
        ),
        redirect_stdout(output),
    ):
        HOOK.main()

    content = output.getvalue()
    return json.loads(content) if content else None


def invoke_with_git_c_worktree(
    command: str, parent_branch: str, worktree_branch: str
) -> dict[str, object] | None:
    parent_cwd = "/workspace/main"
    worktree_cwd = "/workspace/feature-worktree"
    event = json.dumps({"cwd": parent_cwd, "tool_input": {"command": command}})
    output = io.StringIO()
    with (
        patch.object(sys, "stdin", io.StringIO(event)),
        patch.object(
            HOOK,
            "current_branch",
            side_effect=lambda cwd: {
                parent_cwd: parent_branch,
                worktree_cwd: worktree_branch,
            }[cwd],
        ),
        redirect_stdout(output),
    ):
        HOOK.main()

    content = output.getvalue()
    return json.loads(content) if content else None


def invoke_with_workdir_and_git_c_target(
    command: str, workdir_branch: str, target_branch: str
) -> dict[str, object] | None:
    workdir = "/workspace/feature-worktree"
    target = "/workspace/protected-worktree"
    event = json.dumps(
        {
            "cwd": "/workspace/main",
            "tool_input": {"command": command, "workdir": workdir},
        }
    )
    output = io.StringIO()
    with (
        patch.object(sys, "stdin", io.StringIO(event)),
        patch.object(
            HOOK,
            "current_branch",
            side_effect=lambda cwd: {workdir: workdir_branch, target: target_branch}[cwd],
        ),
        redirect_stdout(output),
    ):
        HOOK.main()

    content = output.getvalue()
    return json.loads(content) if content else None


class ProtectBranchesTest(unittest.TestCase):
    def test_denies_git_update_ref_on_protected_branch(self) -> None:
        result = invoke("git update-ref HEAD deadbeef", "develop")

        self.assertEqual(
            result,
            {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "deny",
                    "permissionDecisionReason": "develop は保護ブランチです。Codex から変更できません。",
                }
            },
        )

    def test_allows_git_update_ref_on_feature_branch(self) -> None:
        self.assertIsNone(invoke("git update-ref HEAD deadbeef", "feature/example"))

    def test_allows_mutation_in_feature_worktree_when_parent_is_protected(self) -> None:
        self.assertIsNone(
            invoke_in_worktree("git add api/openapi.yaml", "develop", "feature/example")
        )

    def test_denies_mutation_in_protected_worktree(self) -> None:
        result = invoke_in_worktree("git add api/openapi.yaml", "feature/example", "develop")

        self.assertIsNotNone(result)
        self.assertEqual(
            result["hookSpecificOutput"]["permissionDecision"], "deny"
        )

    def test_allows_git_c_feature_worktree_when_hook_event_omits_workdir(self) -> None:
        self.assertIsNone(
            invoke_with_git_c_worktree(
                "git -C /workspace/feature-worktree add api/openapi.yaml",
                "develop",
                "feature/example",
            )
        )

    def test_denies_compound_git_c_command_targeting_protected_worktree(self) -> None:
        result = invoke_with_git_c_worktree(
            "git -C /workspace/feature-worktree commit --allow-empty -m test && echo done",
            "feature/example",
            "develop",
        )

        self.assertIsNotNone(result)
        self.assertEqual(
            result["hookSpecificOutput"]["permissionDecision"], "deny"
        )

    def test_denies_git_c_protected_target_from_feature_worktree(self) -> None:
        result = invoke_with_workdir_and_git_c_target(
            "git -C /workspace/protected-worktree commit --allow-empty -m test",
            "feature/example",
            "develop",
        )

        self.assertIsNotNone(result)
        self.assertEqual(
            result["hookSpecificOutput"]["permissionDecision"], "deny"
        )

    def test_allows_creating_feature_branch_from_develop(self) -> None:
        self.assertIsNone(invoke("git switch -c feature/example develop", "develop"))

    def test_allows_checkout_branch_from_develop(self) -> None:
        self.assertIsNone(invoke("git checkout -b feature/example develop", "develop"))

    def test_allows_creating_worktree_branch_from_develop(self) -> None:
        self.assertIsNone(
            invoke(
                "git worktree add -b feature/example /tmp/example develop",
                "develop",
            )
        )

    def test_allows_read_only_git_commands_on_protected_branch(self) -> None:
        self.assertIsNone(invoke("git diff --check", "develop"))

    def test_denies_compound_command_after_branch_creation(self) -> None:
        result = invoke(
            "git switch -c feature/example develop && git commit --allow-empty -m test",
            "develop",
        )

        self.assertIsNotNone(result)
        self.assertEqual(result["hookSpecificOutput"]["permissionDecision"], "deny")

    def test_denies_explicit_protected_branch_reference(self) -> None:
        result = invoke("git push origin HEAD:develop", "feature/example")

        self.assertEqual(
            result["hookSpecificOutput"]["permissionDecision"], "deny")

    def test_allows_pull_request_with_protected_base(self) -> None:
        self.assertIsNone(invoke("gh pr create --base develop --head feature/example", "feature/example"))

    def test_denies_github_pull_request_merge_with_unknown_base(self) -> None:
        result = invoke("gh pr merge 123 --merge", "feature/example")

        self.assertIsNotNone(result)
        self.assertEqual(result["hookSpecificOutput"]["permissionDecision"], "deny")

    def test_denies_github_pull_request_merge_api_with_unknown_base(self) -> None:
        result = invoke("gh api repos/example/repo/pulls/123/merge -X PUT", "feature/example")

        self.assertIsNotNone(result)
        self.assertEqual(result["hookSpecificOutput"]["permissionDecision"], "deny")

    def test_denies_github_pull_request_merge_api_with_query(self) -> None:
        result = invoke(
            "gh api repos/example/repo/pulls/123/merge?merge_method=squash -X PUT",
            "feature/example",
        )

        self.assertIsNotNone(result)
        self.assertEqual(result["hookSpecificOutput"]["permissionDecision"], "deny")


if __name__ == "__main__":
    unittest.main()
