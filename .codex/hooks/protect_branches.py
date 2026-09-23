#!/usr/bin/env python3
"""Deny Codex shell commands that operate on protected Git branches.

This protects commands directly issued through Codex's Bash tool. Repository
branch protection on GitHub remains the authoritative protection against work
performed outside Codex or commands intentionally designed to evade this check.
"""

from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
import sys
from typing import Any


PROTECTED_BRANCHES = {"main", "develop"}
BRANCH_REFERENCE = re.compile(
    r"(?<![A-Za-z0-9_.-])(?:refs/heads/|(?:origin|upstream)/)?(?:main|develop)(?![A-Za-z0-9_.-])"
)
BRANCH_CREATION_FROM_PROTECTED = re.compile(
    r"^\s*git\s+(?:"
    r"switch\s+(?:-c|--create)\s+\S+\s+"
    r"|checkout\s+-b\s+\S+\s+"
    r"|worktree\s+add\s+-b\s+\S+\s+\S+\s+"
    r")"
    r"(?:refs/heads/|(?:origin|upstream)/)?(?:main|develop)\s*$"
)
READ_ONLY_GIT_COMMAND = re.compile(
    r"^\s*git\s+(?:"
    r"status(?:\s+--short)?|diff(?:\s+--check)?|log|show|"
    r"rev-parse(?:\s+--show-toplevel)?|remote\s+-v|"
    r"branch(?:\s+--(?:show-current|list|-all|-remotes|-verbose))?"
    r")\s*$"
)
GIT_COMMAND = re.compile(r"\bgit\b")
SHELL_CONTROL = re.compile(r"[;&|><`$()]")
GIT_C_OPTION = re.compile(r"\bgit(?:\s+\S+)*\s+-C(?:\s|\S)")
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


def git_c_workdir(command: str, parent_cwd: str) -> str | None:
    if SHELL_CONTROL.search(command):
        return None

    try:
        arguments = shlex.split(command)
    except ValueError:
        return None
    if not arguments or arguments[0] != "git":
        return None

    workdirs: list[str] = []
    index = 1
    while index < len(arguments):
        argument = arguments[index]
        if argument == "-C":
            if index + 1 >= len(arguments):
                return None
            workdirs.append(arguments[index + 1])
            index += 2
            continue
        if argument.startswith("-C") and len(argument) > 2:
            workdirs.append(argument[2:])
            index += 1
            continue
        if argument.startswith("-"):
            index += 1
            continue
        break

    if len(workdirs) != 1:
        return None
    if os.path.isabs(workdirs[0]):
        return os.path.normpath(workdirs[0])
    return os.path.normpath(os.path.join(parent_cwd, workdirs[0]))


def execution_cwd(event: dict[str, Any], tool_input: dict[str, Any]) -> str | None:
    workdir = tool_input.get("workdir")
    cwd = event.get("cwd")
    if isinstance(workdir, str) and workdir:
        cwd = workdir
    if not isinstance(cwd, str) or not cwd:
        return None
    return git_c_workdir(tool_input.get("command", ""), cwd) or cwd


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

    if SHELL_CONTROL.search(command) and GIT_C_OPTION.search(command):
        deny("git -C を含む複合コマンドは対象ブランチを安全に判定できないため、Codex からは禁止されています。")
        return 0

    branch_creation = BRANCH_CREATION_FROM_PROTECTED.fullmatch(command)
    read_only = READ_ONLY_GIT_COMMAND.fullmatch(command)
    safe_on_protected_branch = branch_creation or read_only

    if (
        BRANCH_REFERENCE.search(command)
        and GIT_COMMAND.search(command)
        and not safe_on_protected_branch
    ):
        deny("main と develop は保護ブランチです。Codex からは操作できません。")
        return 0

    if GH_PULL_REQUEST_MERGE.search(command):
        deny("Pull Request のマージは対象ブランチを安全に判定できないため、Codex からは禁止されています。")
        return 0

    cwd = execution_cwd(event, tool_input)
    if cwd and GIT_COMMAND.search(command) and not safe_on_protected_branch:
        branch = current_branch(cwd)
        if branch in PROTECTED_BRANCHES:
            deny(f"{branch} は保護ブランチです。Codex から変更できません。")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
