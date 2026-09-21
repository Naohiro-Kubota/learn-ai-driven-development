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

    def test_denies_explicit_protected_branch_reference(self) -> None:
        result = invoke("git push origin HEAD:develop", "feature/example")

        self.assertEqual(
            result["hookSpecificOutput"]["permissionDecision"], "deny")

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
