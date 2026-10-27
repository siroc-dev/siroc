#!/usr/bin/env python3
"""Pack the staged siroc/ directory into a gzip tarball with Unix modes."""

import os
import sys
import tarfile

EXEC_NAMES = {"siroc-agent", "siroc-panel", "siroc", "install.sh"}


def mode_for(info: tarfile.TarInfo) -> int:
    name = info.name.replace("\\", "/")
    base = os.path.basename(name)
    if info.isdir():
        return 0o755
    if base in EXEC_NAMES or "/bin/" in "/" + name:
        return 0o755
    return 0o644


def fix(info: tarfile.TarInfo) -> tarfile.TarInfo:
    info.name = info.name.replace("\\", "/")
    info.mode = mode_for(info)
    info.uid = 0
    info.gid = 0
    info.uname = "root"
    info.gname = "root"
    return info


def main(argv: list[str]) -> None:
    if len(argv) != 3:
        raise SystemExit("usage: pack-tarball.py <stage-dir-containing-siroc> <outfile.tar.gz>")
    stage, out = argv[1], argv[2]
    src = os.path.join(stage, "siroc")
    if not os.path.isdir(src):
        raise SystemExit("missing staged directory: " + src)
    parent = os.path.dirname(os.path.abspath(out))
    if parent:
        os.makedirs(parent, exist_ok=True)
    with tarfile.open(out, "w:gz", format=tarfile.GNU_FORMAT) as tar:
        tar.add(src, arcname="siroc", filter=fix)
    print("Wrote", out)


if __name__ == "__main__":
    main(sys.argv)
