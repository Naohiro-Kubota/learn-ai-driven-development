#!/usr/bin/env python3
"""Deny Codex shell commands that operate on protected Git branches.

This protects commands directly issued through Codex's Bash tool. Repository
branch protection on GitHub remains the authoritative protection against work
performed outside Codex or commands intentionally designed to evade this check.
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from typing import Any


PROTECTED_BRANCHES = {"main", "develop"}
BRANCH_REFERENCE = re.compile(
    r"(?<![A-Za-z0-9_.-])(?:refs/heads/|(?:origin|upstream)/)?(?:main|develop)(?![A-Za-z0-9_.-])"
)
GIT_COMMAND = re.compile(r"\bgit\b")
GH_PULL_REQUEST_MERGE = re.compile(
    r"\bgh\s+pr\s+merge\b|\bgh\s+api\b[^\n]*/pulls/[^/\s]+/merge(?:[/?\s]|$)"
)


def current_branch(cwd: str) -> str | None:
    result = subprocess.run(
        ["git", "-C", cwd, "branch", "--show-current"],
        capture_output=True,
        check=False,
        text=True,
    )
    branch = result.stdout.strip()
    return branch or None


def deny(reason: str) -> None:
    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "deny",
                    "permissionDecisionReason": reason,
                }
            }
        )
    )


def main() -> int:
    try:
        event: dict[str, Any] = json.load(sys.stdin)
    except json.JSONDecodeError:
        # Do not make hook-input failures silently disable the protection.
        deny("保護ブランチの判定に必要なフック入力を解析できませんでした。")
        return 0

    tool_input = event.get("tool_input")
    command = tool_input.get("command", "") if isinstance(tool_input, dict) else ""
    if not isinstance(command, str):
        deny("保護ブランチの判定に必要なコマンド入力を解析できませんでした。")
        return 0

    if BRANCH_REFERENCE.search(command) and GIT_COMMAND.search(command):
        deny("main と develop は保護ブランチです。Codex からは操作できません。")
        return 0

    if GH_PULL_REQUEST_MERGE.search(command):
        deny("Pull Request のマージは対象ブランチを安全に判定できないため、Codex からは禁止されています。")
        return 0

    cwd = event.get("cwd")
    if isinstance(cwd, str) and GIT_COMMAND.search(command):
        branch = current_branch(cwd)
        if branch in PROTECTED_BRANCHES:
            deny(f"{branch} は保護ブランチです。Codex から変更できません。")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
