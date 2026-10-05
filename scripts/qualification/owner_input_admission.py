"""Separate each owner's source authority from a shared resource reservation.

The shared plan fixes command paths, USER units and limits. An owner's adopted
record fixes its own bytes. Reading another owner's source is deliberately not
part of resource observation: an unstarted owner can be prepared again without
invalidating a healthy peer. Existing generation, ancestry, OOM, floor and
joined-wait checks remain the resource observer's responsibility.
"""
import json
import os
from pathlib import Path, PurePosixPath
import subprocess

from cargo_control import require
from joint_peer_admission import pin, pinned_json


class OwnerInputAdmission:
    """Only the caller's immutable adoption/job/runner bytes confer admission."""

    def __init__(self, job_path, runner_path, adoption_path, role):
        self.adoption = pin(adoption_path)
        adopted = pinned_json(self.adoption)
        self.plan_pin = adopted["focused_pair_admission"]
        self.plan = pinned_json(self.plan_pin)
        require(self.plan["schema"] == "urnetwork-owner-local-input-cohort-v1",
                "owner-local plan schema differs")
        require(role in self.plan["members"], "caller role is not selected")
        self.role = role
        for member in self.plan["members"].values():
            require(not ({"job", "runner", "job_sha256", "runner_sha256"} & set(member)),
                    "shared resource member must not carry source byte pins")
            require(member["argv"] == [member["python_argv"], member["runner_path"], member["job_path"]],
                    "shared member command differs")
            require(member["uid"] == 1000 and member["manager"] == "user" and
                    member["cgroup"] == "/user.slice/user-1000.slice/user@1000.service/app.slice/" + member["unit"],
                    "shared member USER namespace differs")
        member = self.plan["members"][role]
        require(str(job_path) == member["job_path"] and str(runner_path) == member["runner_path"] and
                str(adoption_path) == member["adoption_path"], "caller path selection differs")
        self.job_pin = {"path": str(job_path), "sha256": adopted["job_sha256"]}
        self.runner_pin = {"path": str(runner_path), "sha256": adopted["runner_sha256"]}
        job = pinned_json(self.job_pin)
        require(pin(runner_path) == self.runner_pin, "caller runner bytes differ")
        require(job["adoption_path"] == str(adoption_path) and job["systemd_unit"] == member["unit"],
                "caller job identity differs")
        require(job["execution_limits"] == member["execution_limits"] and
                job["execution_limits"]["MemoryMax"] == member["memory_max"] and
                job["execution_limits"]["MemorySwapMax"] == 0,
                "caller execution limits differ")
        self.source = job.get("source_admission")
        self.verify()

    def verify(self):
        require(pin(self.plan_pin["path"]) == self.plan_pin, "shared resource plan changed")
        require(pin(self.adoption["path"]) == self.adoption, "caller adoption changed")
        require(pin(self.job_pin["path"]) == self.job_pin, "caller job changed")
        require(pin(self.runner_pin["path"]) == self.runner_pin, "caller runner changed")
        if self.source is not None:
            require(pin(self.source["path"]) == self.source, "caller source authority changed")
        return {"plan": self.plan_pin, "role": self.role, "job": self.job_pin,
                "runner": self.runner_pin, "adoption": self.adoption, "source": self.source}


def _selected(path, spec):
    """Direct package files plus explicitly closed native/embed subtrees."""
    p = PurePosixPath(path)
    require(not p.is_absolute() and ".." not in p.parts, "invalid changed source path")
    return (path in spec["input_files"] or str(p.parent) in spec["package_directories"] or
            any(root == "." or path == root or path.startswith(root + "/")
                for root in spec["input_trees"]))


def verify_git_input_scope(spec):
    """Verify a caller-authenticated complete package/input scope, not HEAD.

    This does not discover dependencies. The admitted caller must provide the
    complete selected package graph and all module, native, embedded, generated
    and configuration inputs. Changes outside that scope cannot qualify a new
    dependency graph. Tool/environment custody remains with ChildContext.
    """
    require(spec["schema"] == "urnetwork-selected-git-input-scope-v1" and
            spec["complete_input_scope"] is True, "complete source scope is not admitted")
    repository = Path(spec["repository"])
    resolved = repository.resolve(strict=True)
    require(str(resolved) == spec["resolved_repository"], "local dependency path changed")
    info = resolved.stat()
    require((info.st_dev, info.st_ino) == (spec["device"], spec["inode"]),
            "local dependency filesystem or inode changed")
    def git(*args):
        result = subprocess.check_output(["/usr/bin/git", "-C", str(repository), *args], timeout=30)
        require(len(result) <= 8 * 1024 * 1024, "source delta exceeds bound")
        return result.decode()
    actual = git("rev-parse", "HEAD").strip()
    changed = set()
    for args in (("diff", "--name-only", "-z", spec["base_commit"], actual),
                 ("diff", "--name-only", "-z", "HEAD"),
                 ("diff", "--cached", "--name-only", "-z"),
                 ("ls-files", "--others", "--exclude-standard", "-z")):
        changed.update(path for path in git(*args).split("\0") if path)
    affected = sorted(path for path in changed if _selected(path, spec))
    require(not affected, "selected build inputs changed: " + repr(affected[:16]))
    return {"repository": str(repository), "base_commit": spec["base_commit"],
            "observed_head": actual, "outside_scope_changes": sorted(changed),
            "selected_input_changes": []}
