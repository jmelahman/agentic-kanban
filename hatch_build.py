from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

from hatchling.builders.hooks.plugin.interface import BuildHookInterface

import manygo


class GoBinaryBuildHook(BuildHookInterface):
    def initialize(self, version, build_data) -> None:  # noqa: ANN001, ARG002
        build_data["pure_python"] = False
        goos = os.getenv("GOOS")
        goarch = os.getenv("GOARCH")
        if goos and goarch:
            build_data["tag"] = "py3-none-" + manygo.get_platform_tag(goos=goos, goarch=goarch)  # type: ignore[invalid-argument-type]
        binary_name = self.config["binary_name"]
        tag = os.getenv("GITHUB_REF_NAME", "dev")
        commit = os.getenv("GITHUB_SHA", "none")

        web_dir = Path(self.root) / "web"
        dist_dir = web_dir / "dist"
        if not dist_dir.exists() or not any(dist_dir.iterdir()):
            print("Building frontend...")
            npm = shutil.which("npm")
            if npm is None:
                raise RuntimeError("npm is required to build the kanban frontend")
            install_cmd = "ci" if (web_dir / "package-lock.json").exists() else "install"
            subprocess.check_call([npm, install_cmd], cwd=web_dir)  # noqa: S603
            subprocess.check_call([npm, "run", "build"], cwd=web_dir)  # noqa: S603

        if not os.path.exists(binary_name):
            print(f"Building Go binary '{binary_name}'...")
            ldflags = (
                f"-X github.com/jmelahman/kanban/cmd/server.version={tag} "
                f"-X github.com/jmelahman/kanban/cmd/server.commit={commit} "
                "-s -w"
            )
            subprocess.check_call(  # noqa: S603
                [
                    "go",
                    "build",
                    "-tags=embed",
                    "-trimpath",
                    f"-ldflags={ldflags}",
                    "-o",
                    binary_name,
                ],
            )

        build_data["shared_scripts"] = {binary_name: binary_name}
